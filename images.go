package main

import (
	"fmt"
	"math/rand/v2"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
)

func main() {
	path, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	entries, err := contents(path, 100000)
	if err != nil {
		panic(err)
	}

	if s, ok := os.LookupEnv("SORT"); !ok || s == "random" {
		rand.Shuffle(len(entries), func(i, j int) { entries[i], entries[j] = entries[j], entries[i] })
	} else if strings.HasSuffix(s, "time") {
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].ModTime().Before(entries[j].ModTime())
		})
	} else if strings.HasPrefix(s, "new") {
		sort.Slice(entries, func(i, j int) bool {
			return entries[j].ModTime().Before(entries[i].ModTime())
		})
	}

	if limitString := os.Getenv("LIMIT"); limitString != "" {
		if limit, err := strconv.Atoi(limitString); err == nil {
			limit = min(limit, len(entries))
			fmt.Println("Limiting to", limit)
			entries = entries[:limit]
		}
	}

	var send, receive = make(chan string, 1), make(chan string, 1)

	var wg = new(sync.WaitGroup)

	numWorkers := 16
	wg.Add(numWorkers)
	for range numWorkers {
		go mimeWorker(wg, send, receive)
	}

	files := make([]string, 0, len(entries))

	go func() {
		wg.Wait()
		close(receive)
	}()

	go func() {
		defer close(send)
		for _, entry := range entries {
			if !entry.IsDir() {
				send <- entry.path
			}
		}
	}()

	for entry := range receive {
		files = append(files, entry)
	}

	fmt.Println("Opening", len(files), "files")
	files = append(os.Args[1:], files...)

	fmt.Println(exec.Command("imv", files...).Run())
}

type fullEntry struct {
	os.FileInfo
	path string
}

func contents(pwd string, recurse int) ([]fullEntry, error) {
	entries, err := os.ReadDir(pwd)
	if err != nil {
		return nil, err
	}

	var allEntries = make([]fullEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			if recurse == 0 {
				continue
			}

			dirEntries, err := contents(pwd+"/"+entry.Name(), recurse-1)
			if err != nil {
				return allEntries, nil
			}

			allEntries = append(allEntries, dirEntries...)
		}

		info, err := entry.Info()
		if err != nil {
			return allEntries, err
		}

		allEntries = append(allEntries, fullEntry{
			FileInfo: info,
			path:     pwd + "/" + entry.Name(),
		})
	}

	return allEntries, nil
}

// mimeWorker reads paths from a channel and tries to detect the mimetype. Emits
// the path on a different channel if the mimetype is an image or detection
// fails.
func mimeWorker(wg *sync.WaitGroup, read <-chan string, write chan<- string) {
	defer wg.Done()

	// The content type exists in the first 512 bytes.
	buf := make([]byte, 512)

	for path := range read {
		output, err := os.Open(path)
		if err == nil {
			_, err = output.Read(buf)
			output.Close()

			contentType := http.DetectContentType(buf)
			if err == nil && !strings.HasPrefix(contentType, "image/") {
				continue
			}
		}

		write <- path
	}
}
