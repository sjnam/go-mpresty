// Command mpresty serves a document root, turning MetaPost and Graphviz
// source embedded in its HTML pages into SVG images.
//
//line mpresty.gw:23
//line mpresty.gw:24
//line mpresty.gw:25
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sjnam/go-mpresty/internal/cache"
	"github.com/sjnam/go-mpresty/internal/render"
	"github.com/sjnam/go-mpresty/internal/server"
)

func main() {

//line mpresty.gw:55
	var (
		addr     = flag.String("addr", ":8080", "address to listen on")
		root     = flag.String("root", "webapp/html", "document root")
		workdir  = flag.String("workdir", "images", "directory for rendered SVGs, relative to root")
		timeout  = flag.Duration("timeout", 30*time.Second, "wall clock budget for one engine run")
		maxJobs  = flag.Int("jobs", 0, "concurrent engine runs (0 = GOMAXPROCS)")
		cacheCap = flag.Int("cache", 0, "cache entries to keep (0 = default)")
		verbose  = flag.Bool("v", false, "log at debug level")
	)
	flag.Parse()

//line mpresty.gw:45

//line mpresty.gw:70
	level := slog.LevelInfo
	if *verbose {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(log)

//line mpresty.gw:46

//line mpresty.gw:81
	proc, err := render.New(render.Options{
		Root:    *root,
		WorkDir: *workdir,
		Timeout: *timeout,
		MaxJobs: *maxJobs,
		Cache:   cache.New(*cacheCap),
		Logger:  log,
	})
	if err != nil {

//line mpresty.gw:100
		log.Error("mpresty stopped", "err", err)
		os.Exit(1)

//line mpresty.gw:91
	}
	srv, err := server.New(*root, proc, log)
	if err != nil {

//line mpresty.gw:100
		log.Error("mpresty stopped", "err", err)
		os.Exit(1)

//line mpresty.gw:95
	}

//line mpresty.gw:47

//line mpresty.gw:112
	httpd := &http.Server{
		Addr:              *addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	errc := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", *addr, "root", *root)
		errc <- httpd.ListenAndServe()
	}()

//line mpresty.gw:133
	select {
	case err = <-errc:
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
	case <-ctx.Done():
		log.Info("shutting down")
		sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		err = httpd.Shutdown(sctx)
		cancel()
	}

//line mpresty.gw:125
	if err != nil {

//line mpresty.gw:100
		log.Error("mpresty stopped", "err", err)
		os.Exit(1)

//line mpresty.gw:127
	}

//line mpresty.gw:48
}
