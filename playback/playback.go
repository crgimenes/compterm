package playback

import (
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"time"
)

type Data struct {
	CMD     byte
	Payload []byte
	Time    int64
}

type Playback struct {
	FileName string
	f        *os.File
	writer   *csv.Writer
	reader   *csv.Reader
}

func New(fileName string) *Playback {
	p := &Playback{
		FileName: fileName,
	}

	return p
}

func (p *Playback) Open() error {
	var err error
	p.f, err = os.Open(p.FileName)
	if err != nil {
		log.Fatalf("error opening file: %s\r\n", err)
	}

	p.reader = csv.NewReader(p.f)
	p.reader.Comma = ';'
	p.reader.FieldsPerRecord = 3
	p.reader.LazyQuotes = true

	return nil
}

func (p *Playback) OpenToAppend() error {
	var err error
	p.f, err = os.OpenFile(p.FileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("error opening file: %s\r\n", err)
	}

	p.writer = csv.NewWriter(p.f)
	p.writer.Comma = ';'

	return nil
}

func (p *Playback) Close() error {
	p.writer.Flush()
	return p.f.Close()
}

// rec data im csv format
func (p *Playback) Rec(cmd byte, payload []byte) error {
	data := []string{
		fmt.Sprintf("%d", cmd),
		string(payload),
		fmt.Sprintf("%d", time.Now().UnixNano()),
	}
	err := p.writer.Write(data)
	return err
}

func (p *Playback) Play(w io.Writer) error {
	data := Data{}
	lastTime := int64(0)

	log.Printf("start playback\r\n")

	i := 0

	for {

		log.Printf("playback %d\r\n", i)

		// TODO: add a way to stop the playback

		// Read one record (until the next delimiter) from csvReader.
		record, err := p.reader.Read()
		if err != nil {
			if err == io.EOF {
				log.Printf("end of file\r\n")
				break
			}
			log.Printf("error reading record to file: %s\r\n", err)

			return fmt.Errorf("error reading record to file: %s\r\n", err)
		}

		c, err := strconv.ParseInt(record[0], 10, 8)
		if err != nil {
			log.Printf("error parsing record to file: %s\r\n", err)
			return fmt.Errorf("error parsing record to file: %s\r\n", err)
		}

		data.CMD = byte(c)

		data.Payload = []byte(record[1])
		t, err := strconv.ParseInt(record[2], 10, 64)
		if err != nil {
			log.Printf("error parsing record to file: %s\r\n", err)
			return fmt.Errorf("error parsing record to file: %s\r\n", err)
		}

		data.Time = t

		if lastTime == 0 {
			lastTime = data.Time
		}

		// wait to send
		time.Sleep(time.Duration(data.Time-lastTime) * time.Nanosecond)

		lastTime = data.Time

		// send data
		n := len(data.Payload) + 1
		buffer := make([]byte, n)
		buffer[0] = data.CMD
		copy(buffer[1:], data.Payload)

		_, err = w.Write(buffer[:n])
		if err != nil {
			log.Printf("error writing to output: %s\r\n", err)
			return fmt.Errorf("error writing to output: %s\r\n", err)
		}

		log.Printf("playback %d done\r\n", i)
		i++
	}

	return nil
}
