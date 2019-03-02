package main

import (
	"io"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/nareix/joy4/av/avutil"
	"github.com/nareix/joy4/av/pktque"
	"github.com/nareix/joy4/av/pubsub"
	"github.com/nareix/joy4/format"
	"github.com/nareix/joy4/format/flv"
	"github.com/nareix/joy4/format/rtmp"
	"github.com/nareix/joy4/format/rtsp"
	_ "github.com/nareix/joy4/pprof"
)

func init() {
	//rtmp.Debug = true
	format.RegisterAll()
}

type writeFlusher struct {
	httpflusher http.Flusher
	io.Writer
}

func (self writeFlusher) Flush() error {
	self.httpflusher.Flush()
	return nil
}

func main() {
	server := &rtmp.Server{}

	type Channel struct {
		que *pubsub.Queue
	}
	channels := &sync.Map{}

	server.HandlePlay = func(conn *rtmp.Conn) {
		log.Printf("play:%s\n", conn.URL.Path)

		if _ch, ok := channels.Load(conn.URL.Path); ok {
			ch := _ch.(*Channel)
			cursor := ch.que.Latest()
			avutil.CopyFile(conn, cursor)
		}
	}

	server.HandlePublish = func(conn *rtmp.Conn) {

		log.Printf("publish:%s\n", conn.URL.Path)
		streams, _ := conn.Streams()

		ch := &Channel{}
		ch.que = pubsub.NewQueue()
		ch.que.WriteHeader(streams)

		_, ok := channels.LoadOrStore(conn.URL.Path, ch)
		if ok {
			log.Printf("exsit %s\n", conn.URL.Path)
			conn.Close()
			return
		}

		avutil.CopyPackets(ch.que, conn)

		channels.Delete(conn.URL.Path)

		ch.que.Close()
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")

		_path := strings.TrimSuffix(r.URL.Path, ".flv")

		var ch *Channel
		if _ch, ok := channels.Load(_path); ok {
			ch = _ch.(*Channel)
		} else {
			http.NotFound(w, r)
			return
		}

		if ch != nil {
			w.Header().Set("Content-Type", "video/x-flv")
			w.Header().Set("Transfer-Encoding", "chunked")
			w.WriteHeader(200)
			flusher := w.(http.Flusher)
			flusher.Flush()

			muxer := flv.NewMuxerWriteFlusher(writeFlusher{httpflusher: flusher, Writer: w})
			cursor := ch.que.Latest()

			avutil.CopyFile(muxer, cursor)
		} else {
			http.NotFound(w, r)
		}
	})

	go http.ListenAndServe(":7001", nil)

	go func() {
		rt, err := rtmp.Dial("rtmp://ossrs.net:1935/live/livestream")
		if err != nil {
			log.Println(err)
		}
		rm, err := rtmp.Dial("rtmp://localhost/live/test")
		if err != nil {
			log.Println(err)
		}

		demuxer := &pktque.FilterDemuxer{Demuxer: rt, Filter: &pktque.WaitKeyFrame{}}
		avutil.CopyFile(rm, demuxer)

		rt.Close()
		rm.Close()
	}()

	go func() {
		rt, err := rtsp.Dial("rtsp://admin:123456abc@192.168.1.64:554/h264/ch1/main/av_stream")
		if err != nil {
			log.Println(err)
		}
		rm, err := rtmp.Dial("rtmp://localhost/live/test1")
		if err != nil {
			log.Println(err)
		}

		filters := pktque.Filters{}
		filters = append(filters, &pktque.WaitKeyFrame{})
		//filters = append(filters, &pktque.FrameDropper{DelaySkip: 100})
		//filters = append(filters, &pktque.FixTime{MakeIncrement: true})

		demuxer := &pktque.FilterDemuxer{Demuxer: rt, Filter: filters}
		avutil.CopyFile(rm, demuxer)

		rt.Close()
		rm.Close()
	}()

	server.ListenAndServe()

	// ffmpeg -re -i movie.flv -c copy -f flv rtmp://localhost/movie
	// ffmpeg -f avfoundation -i "0:0" .... -f flv rtmp://localhost/screen
	// ffplay http://localhost:8089/movie
	// ffplay http://localhost:8089/screen
}
