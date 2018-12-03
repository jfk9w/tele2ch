package feed

import (
	"encoding/json"
	"strings"

	"github.com/jfk9w-go/gox/mathx"

	"github.com/jfk9w-go/hikkabot/content"

	"sort"

	"os"

	"fmt"
	"time"

	"github.com/jfk9w-go/dvach"
	"github.com/jfk9w-go/red"
	"github.com/jfk9w-go/telegram"
	. "github.com/pkg/errors"
)

type Service interface {
	Load(*State) (Load, error)
	Title(*State) string
}

type GenericService struct {
	Typed map[Type]Service
}

func (service *GenericService) Load(state *State) (Load, error) {
	var concrete, ok = service.Typed[state.Type]
	if ok {
		var load, err = concrete.Load(state)
		if err != nil {
			return &DummyLoad{}, err
		}

		return load, nil
	}

	return &DummyLoad{}, Errorf("unsupported type: %s", state.Type)
}

func (service *GenericService) Title(state *State) string {
	var concrete, ok = service.Typed[state.Type]
	if ok {
		return concrete.Title(state)
	}

	return string(state.Type)
}

type DvachService struct {
	Dvach
	Aconvert
}

func ParseDvachRef(value string) (dvach.Ref, error) {
	var tokens = strings.Split(value, "/")
	if len(tokens) != 2 {
		return dvach.Ref{}, Errorf("invalid thread ID: %s", value)
	}

	return dvach.ToRef(tokens[0], tokens[1])
}

func (service *DvachService) ParseState(state *State) (ref dvach.Ref, meta *DvachMeta, err error) {
	ref, err = ParseDvachRef(state.ID)
	if err != nil {
		return
	}

	meta = new(DvachMeta)
	err = state.ParseMeta(meta)
	return
}

func (service *DvachService) Load(state *State) (Load, error) {
	var (
		ref    dvach.Ref
		offset = state.Offset
		meta   *DvachMeta
		posts  []*dvach.Post
		err    error
	)

	ref, meta, err = service.ParseState(state)
	if err != nil {
		return nil, err
	}

	if offset > 0 {
		offset += 1
	}

	posts, err = service.Dvach.Posts(ref, offset)
	if err != nil {
		return nil, err
	}

	return &DvachLoad{service.Dvach, service.Aconvert, meta, posts, 0}, nil
}

func (service *DvachService) Title(state *State) string {
	var (
		meta = new(DvachMeta)
		err  error
	)

	err = state.ParseMeta(meta)
	if err != nil {
		return string(state.Type)
	}

	return meta.Title
}

type DvachWatchService struct {
	Dvach
}

func (service *DvachWatchService) Load(state *State) (Load, error) {
	var (
		offset  = state.Offset
		meta    = new(DvachWatchMeta)
		catalog *dvach.Catalog
		threads []*dvach.Thread
		results []string
		offsets []int
		err     error
	)

	err = json.Unmarshal(state.Meta, meta)
	if err != nil {
		return nil, err
	}

	catalog, err = service.Catalog(state.ID)
	if err != nil {
		return nil, err
	}

	threads = make([]*dvach.Thread, 0)
	for _, thread := range catalog.Threads {
		if thread.Num > offset {
			threads = append(threads, thread)
		}
	}

	threads = content.SearchDvachCatalog(threads, content.DvachSortByNum, meta.Query, -1)
	results = content.FormatDvachCatalog(threads)
	if len(threads) > 0 {
		offsets = make([]int, len(results))
		for i := range results {
			var idx = mathx.MaxInt(0,
				mathx.MinInt(
					(i+1)*content.DvachPreviewsPerMessage-1,
					len(threads)-1))

			offsets[i] = threads[idx].Num
		}
	}

	return &DvachWatchLoad{results, offsets, 0}, nil
}

func (service *DvachWatchService) Title(state *State) string {
	return "2ch/" + state.ID + " watcher"
}

type RedService struct {
	Red
	MetricsFile   string
	MetricsChatID telegram.ChatID
}

func (service *RedService) Load(state *State) (Load, error) {
	var (
		offset = float32(state.Offset)
		meta   = new(RedMeta)
		data   []red.ThingData
		err    error
	)

	err = state.ParseMeta(meta)
	if err != nil {
		return nil, err
	}

	data, err = service.Listing(state.ID+"/"+meta.Mode, 100)
	if err != nil {
		return nil, err
	}

	sort.Sort(SortRedData(data))

	var metrics *os.File
	if service.MetricsFile != "" && service.MetricsChatID != 0 {
		metrics, err = os.OpenFile(service.MetricsFile, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0660)
		if err != nil {
			log.Warnf("Failed to open file %s: %s", service.MetricsFile, err.Error())
			metrics = nil
		}
	}

	var filtered = make([]red.ThingData, 0)
	var moment = time.Now().Unix()
	for _, datum := range data {
		if metrics != nil {
			metrics.WriteString(fmt.Sprintf("%d,%s,%s,%s,%.0f,%d\n",
				moment, state.ID, meta.Mode, datum.Name, datum.CreatedUTC, datum.Ups))
		}

		if datum.CreatedUTC > offset && datum.Ups > meta.Ups {
			filtered = append(filtered, datum)
		}
	}

	if metrics != nil {
		metrics.Close()
	}

	return &RedLoad{service.Red, filtered, 0}, nil
}

func (service *RedService) Title(state *State) string {
	return state.ID
}

type SortRedData []red.ThingData

func (data SortRedData) Len() int {
	return len(data)
}

func (data SortRedData) Less(i, j int) bool {
	return data[i].CreatedUTC < data[j].CreatedUTC
}

func (data SortRedData) Swap(i, j int) {
	data[i], data[j] = data[j], data[i]
}
