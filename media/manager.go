package media

import (
	"log"
	"sync"
	"time"

	aconvert "github.com/jfk9w-go/aconvert-api"

	"github.com/pkg/errors"
)

type Config struct {
	Concurrency int
	Aconvert    *aconvert.Config
}

type Manager struct {
	convs []Converter
	queue chan *Media
	work  sync.WaitGroup
}

func NewManager(config Config) *Manager {
	if config.Concurrency < 1 {
		panic("concurrency should be greater than 0")
	}
	manager := &Manager{
		convs: []Converter{SupportedFormats},
		queue: make(chan *Media),
	}
	if config.Aconvert != nil {
		aconverter := NewAconverter(*config.Aconvert)
		manager.AddConverter(aconverter)
	}
	for i := 0; i < config.Concurrency; i++ {
		go manager.runWorker()
	}
	return manager
}

func (m *Manager) AddConverter(conv Converter) *Manager {
	m.convs = append(m.convs, conv)
	return m
}

func (m *Manager) Submit(url string, format string, in SizeAwareReadable) *Media {
	media := &Media{
		URL:    url,
		format: format,
		in:     in,
		err:    errors.New("not loaded"),
	}
	if media.in != nil {
		media.work.Add(1)
		m.queue <- media
	}
	return media
}

func (m *Manager) Shutdown() {
	close(m.queue)
	m.work.Wait()
}

func (m *Manager) runWorker() {
	m.work.Add(1)
	defer m.work.Done()
	for media := range m.queue {
		out, err := m.process(media.URL, media.format, media.in)
		if err != nil {
			log.Printf("Failed to process media %s: %s", media.URL, err)
		}
		media.out = out
		media.err = err
		media.work.Done()
	}
}

func (m *Manager) process(url string, format string, in SizeAwareReadable) (*TypeAwareReadable, error) {
	start := time.Now()
	for _, conv := range m.convs {
		in, typ, err := conv.Convert(format, in)
		switch err {
		case nil:
			size, err := in.Size()
			if err != nil {
				return nil, errors.Wrap(err, "size calculation")
			}
			if size < MinMediaSize {
				return nil, errors.Errorf("size (%d bytes) is below minimum size (%d bytes)", size, MinMediaSize)
			}
			if maxSize, ok := MaxMediaSize[typ]; ok && size > maxSize {
				return nil, errors.Errorf("size (%d MB) exceeds limit (%d MB) for type %s", size>>20, maxSize>>20, typ)
			}
			log.Printf("Processed %s %s (%d Kb) via %T in %v", typ, url, size>>10, conv, time.Now().Sub(start))
			return &TypeAwareReadable{Readable: in, Type: typ}, nil
		case UnsupportedTypeErr:
			continue
		default:
			return nil, errors.Wrap(err, "conversion failed")
		}
	}
	return nil, UnsupportedTypeErr
}
