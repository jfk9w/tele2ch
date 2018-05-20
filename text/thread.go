package text

import (
	"fmt"
	"time"

	"strings"

	"github.com/jfk9w-go/dvach"
)

type Thread struct {
	*dvach.Thread
	PostsPerHour float64
}

func searchThreads(threads []*dvach.Thread, searchText []string) []Thread {
	for i, st := range searchText {
		searchText[i] = strings.ToLower(st)
	}

	now := time.Now()
	r := make([]Thread, 0)

main:
	for _, thread := range threads {
		date, ok := dvach.ToTime(thread.DateString)
		if !ok {
			continue
		}

		if len(searchText) > 0 {
			comment := strings.ToLower(thread.Comment)
			for _, st := range searchText {
				if !strings.Contains(comment, st) {
					continue main
				}
			}
		}

		age := now.Sub(date).Hours()
		r = append(r, Thread{thread, float64(thread.PostsCount) / age})
	}

	return r
}

func FormatThread(thread Thread) string {
	chunks := format(thread.Item, 275)
	if len(chunks) == 0 {
		return ""
	}

	header := fmt.Sprintf("<b>%s</b>\n%s\n%d / %.2f/hr\n---\n",
		thread.DateString, FormatRef(thread.Ref), thread.PostsCount, thread.PostsPerHour)

	return header + chunks[0]
}
