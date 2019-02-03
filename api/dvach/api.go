package dvach

import (
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"
)

const (
	Domain = "2ch.hk"
	Host   = "https://" + Domain
)

type FileType int

const (
	JPEG FileType = 1
	PNG  FileType = 2
	GIF  FileType = 4
	WebM FileType = 6
	MP4  FileType = 10
)

type File struct {
	Path         string   `json:"path"`
	Type         FileType `json:"type"`
	Size         int      `json:"size"`
	DurationSecs *int     `json:"duration_secs"`
	Width        *int     `json:"width"`
	Height       *int     `json:"height"`
}

type Post struct {
	NumString    string  `json:"num"`
	ParentString string  `json:"parent"`
	DateString   string  `json:"date"`
	Subject      string  `json:"subject"`
	Comment      string  `json:"comment"`
	Files        []*File `json:"files"`

	// OP-only fields
	PostsCount *int `json:"posts_count"`
	FilesCount *int `json:"files_count"`

	// fields with custom initialization
	BoardID string
	Num     int
	Parent  int
	Date    time.Time
}

var (
	tz     *time.Location
	tzOnce sync.Once
)

func (post *Post) init(boardID string) (err error) {
	tzOnce.Do(func() {
		loc, err := time.LoadLocation("Europe/Moscow")
		if err != nil {
			panic(err)
		}

		tz = loc
	})

	post.BoardID = boardID
	post.Num, err = strconv.Atoi(post.NumString)
	if err != nil {
		return
	}

	post.Parent, err = strconv.Atoi(post.ParentString)
	if err != nil {
		return
	}

	if post.Parent == 0 {
		post.Parent = post.Num
		post.ParentString = post.NumString
	}

	var dateString = []rune(post.DateString)
	post.Date, err = time.ParseInLocation("02/01/06 15:04:05",
		string(dateString[:8])+string(dateString[12:]), tz)

	return err
}

func (post *Post) IsOriginal() bool {
	return post.Parent == post.Num
}

func (post *Post) URL() string {
	if post.IsOriginal() {
		return fmt.Sprintf("%s/%s/res/%s.html", Host, post.BoardID, post.NumString)
	}

	return fmt.Sprintf("%s/%s/res/%s.html#%s", Host, post.BoardID, post.ParentString, post.NumString)
}

type posts []*Post

func (posts posts) init(boardID string) (err error) {
	for _, post := range posts {
		err = post.init(boardID)
		if err != nil {
			return
		}
	}

	return
}

type Catalog struct {
	Threads []*Post `json:"threads"`
}

func (catalog *Catalog) init(boardID string) (err error) {
	return posts(catalog.Threads).init(boardID)
}

type Board struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Error struct {
	Code int    `json:"Code"`
	Err  string `json:"Error"`
}

func (e *Error) Error() string {
	return fmt.Sprintf("%d %s", e.Code, e.Err)
}

func cookies(usercode string, path string) []*http.Cookie {
	return []*http.Cookie{
		{
			Name:   "usercode_auth",
			Value:  usercode,
			Domain: Domain,
			Path:   path,
		},
		{
			Name:   "ageallow",
			Value:  "1",
			Domain: Domain,
			Path:   path,
		},
	}
}
