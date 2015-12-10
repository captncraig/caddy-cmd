package cmd

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/mholt/caddy/middleware"
)

type cmdModule struct {
	next     middleware.Handler
	root     string
	Commands []*command
	uiPath   string
}

type command struct {
	Description     string
	Path            string
	Execs           []*ex
	Timeout         time.Duration
	Method          string
	AllowConcurrent bool
	lock            chan bool
}

type ex struct {
	Command string
	Args    []string
}

func (c *cmdModule) ServeHTTP(w http.ResponseWriter, r *http.Request) (int, error) {
	if r.URL.Path == c.uiPath {
		buf := &bytes.Buffer{}
		if err := tmpl.Execute(buf, c); err != nil {
			return http.StatusInternalServerError, err
		}
		w.Header().Add("Content-Type", "text/html")
		w.Write(buf.Bytes())
		return 200, nil
	}
	for _, cmd := range c.Commands {
		if cmd.Method == r.Method && middleware.Path(r.URL.Path).Matches(cmd.Path) {
			return cmd.Execute(w, c.root)
		}
	}
	return c.next.ServeHTTP(w, r)
}

func (c *command) Execute(w http.ResponseWriter, root string) (int, error) {
	// This looks a bit ugly, but it does a lot of fancy things.
	// 1. Ensures only one request at a time can execute command unless multiple is set.
	// 2. Run command with timeout. Kill after timeout.
	// 3. Wrap writer with one that flushes on every write.
	// Possible enhancement: option to kill process on client disconnect. Do not want to enable by default
	// because a webhook may want to be "fire and forget"
	if !c.AllowConcurrent {
		//try lock
		select {
		case c.lock <- true:
		default:
			return http.StatusConflict, fmt.Errorf("Already running")
		}
	}

	fw := flushWriter{w: w}
	if f, ok := w.(http.Flusher); ok {
		fw.f = f
	}
	// maybe only do this if we decide they are a browser
	w.Header().Add("Content-Type", "text/html")
	fmt.Fprint(w, "<pre>")
Loop:
	for _, exe := range c.Execs {
		fmt.Fprintf(w, "Executing %s %s\n", exe.Command, strings.Join(exe.Args, " "))
		cmd := exec.Command(exe.Command, exe.Args...)
		cmd.Stdout = &fw
		cmd.Stderr = &fw
		cmd.Dir = root
		cmd.Start()
		done := make(chan error, 1)
		go func() {
			done <- cmd.Wait()
		}()
		select {
		case <-time.After(c.Timeout):
			if err := cmd.Process.Kill(); err != nil {
				fmt.Fprintf(w, "failed to kill: ", err)
				break Loop
			}
			fmt.Fprintf(w, "Timeout. Killed.\n")
			break Loop
		case err := <-done:
			if err != nil {
				fmt.Fprintf(w, "process done with error = %v", err)
				break Loop
			}
		}
		fmt.Fprint(w, "\n")
	}

	if !c.AllowConcurrent {
		<-c.lock
	}
	return 200, nil
}

type flushWriter struct {
	f http.Flusher
	w io.Writer
}

func (fw *flushWriter) Write(p []byte) (n int, err error) {
	n, err = fw.w.Write(p)
	if fw.f != nil {
		fw.f.Flush()
	}
	return
}
