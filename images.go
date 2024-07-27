package main

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	videoProgram = env("VIDEO_PROGRAM", "vlc")
	imageProgram = env("IMAGE_PROGRAM", "imv")

	programToMime = map[string][]string{
		videoProgram: {"video/", "application/octet-stream"},
		imageProgram: {"image/"},
	}

	program = imageProgram
	start   = time.Now()
)

func env(key, fb string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}

	return fb
}

func main() {
	if strings.HasPrefix(os.Args[0], "video") {
		program = "vlc"
	}

	path, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	var recurse = 1000
	if _, ok := os.LookupEnv("DONT_RECURSE"); ok {
		recurse = 0
	}

	var send, receive = make(chan fullEntry), make(chan fullEntry)
	var wg = new(sync.WaitGroup)

	numWorkers := 16
	wg.Add(numWorkers)
	for range numWorkers {
		go mimeWorker(wg, send, receive)
	}

	// Prepare the receiving of files.
	entries := make([]fullEntry, 0, 100000)

	// Close the receiving channel after checking the entire directory.
	go func() {
		wg.Wait()
		close(receive)
	}()

	// Rad the results
	go func() {
		for entry := range receive {
			entries = append(entries, entry)
		}
	}()

	// Retrieve all contents of the directory.
	if err := contents(path, recurse, send); err != nil {
		panic(err)
	}

	// Sent all files, close the sending channel.
	close(send)

	if s, ok := os.LookupEnv("SORT"); !ok || s == "random" {
		fmt.Println("Sorting randomly")
		rand.Shuffle(len(entries), func(i, j int) { entries[i], entries[j] = entries[j], entries[i] })
	} else if strings.HasSuffix(s, "time") {
		fmt.Println("Sorting by oldest first")
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].ModTime().Before(entries[j].ModTime())
		})
	} else if strings.HasPrefix(s, "new") {
		fmt.Println("Sorting by newest first")
		sort.Slice(entries, func(i, j int) bool {
			return entries[j].ModTime().Before(entries[i].ModTime())
		})
	}

	var upperBound = len(entries)
	if limitString := os.Getenv("LIMIT"); limitString != "" {
		if limit, err := strconv.Atoi(limitString); err == nil {
			upperBound = min(limit, upperBound)
		}
	}

	files := make([]string, upperBound)
	for i := range upperBound {
		files[i] = entries[i].path
	}

	fmt.Printf("Found %v files, took %v. Will open files using '%v'.\n", len(files), time.Since(start), program)
	if len(files) == 0 {
		fmt.Println("No files to open, exiting.")
		return
	}

	files = append(os.Args[1:], files...)
	if err := exec.Command(program, files...).Run(); err != nil {
		fmt.Println("Error running", program)
		var exitErr exec.ExitError
		if errors.As(err, &err) {
			os.Exit(exitErr.ExitCode())
		}
	}
}

type fullEntry struct {
	os.FileInfo
	path string
}

func contents(pwd string, recurse int, send chan<- fullEntry) error {
	entries, err := os.ReadDir(pwd)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			if recurse == 0 {
				continue
			}

			if err := contents(pwd+"/"+entry.Name(), recurse-1, send); err != nil {
				return err
			}
		}

		info, err := entry.Info()
		if err != nil {
			return err
		}

		send <- fullEntry{
			FileInfo: info,
			path:     pwd + "/" + entry.Name(),
		}
	}

	return nil
}

// mimeWorker reads paths from a channel and tries to detect the mimetype. Emits
// the path on a different channel if the mimetype is an image or detection
// fails.
func mimeWorker(wg *sync.WaitGroup, read <-chan fullEntry, write chan<- fullEntry) {
	defer wg.Done()

	// The content type exists in the first 512 bytes.
	buf := make([]byte, 512)

outer:
	for fe := range read {
		output, err := os.Open(fe.path)
		if err == nil {
			_, err = output.Read(buf)
			_ = output.Close()

			contentType := http.DetectContentType(buf)

			for _, mime := range programToMime[program] {
				if err == nil && strings.HasPrefix(contentType, mime) {
					write <- fe
					continue outer
				}
			}
		}
	}
}
