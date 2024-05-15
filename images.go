package main

import (
	"fmt"
	"math/rand/v2"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

func main() {
	path, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		panic(err)
	}

	if s, ok := os.LookupEnv("SORT"); !ok || s == "random" {
		rand.Shuffle(len(entries), func(i, j int) { entries[i], entries[j] = entries[j], entries[i] })
	} else if strings.HasSuffix(s, "time") {
		sort.Slice(entries, func(i, j int) bool {
			iInfo, err := entries[i].Info()
			if err != nil {
				panic(err)
			}

			jInfo, err := entries[j].Info()
			if err != nil {
				panic(err)
			}

			return iInfo.ModTime().Before(jInfo.ModTime())
		})
	} else if strings.HasPrefix(s, "new") {
		sort.Slice(entries, func(i, j int) bool {
			iInfo, err := entries[i].Info()
			if err != nil {
				panic(err)
			}

			jInfo, err := entries[j].Info()
			if err != nil {
				panic(err)
			}

			return jInfo.ModTime().Before(iInfo.ModTime())
		})
	}

	if limitString := os.Getenv("LIMIT"); limitString != "" {
		if limit, err := strconv.Atoi(limitString); err == nil {
			limit = min(limit, len(entries))
			fmt.Println("Limiting to", limit)
			entries = entries[:limit]
		}
	}

	files := make([]string, 0, len(entries)+10)

	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			panic(err)
		}

		if info.IsDir() {
			continue
		}

		files = append(files, path+"/"+info.Name())
	}

	files = append(os.Args[1:], files...)

	fmt.Println(exec.Command("imv", files...).Run())
}
