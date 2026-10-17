package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"

	bkclient "github.com/moby/buildkit/client"
	"github.com/moby/moby/api/types/build"
	"github.com/moby/moby/client"
	"github.com/opencontainers/go-digest"
	"github.com/tonistiigi/fsutil"

	"github.com/SiddheshNan/moonshed/internal/archive"
)

// Build builds the Dockerfile in dir and tags the image. It uses the engine's BuildKit,
// and only falls back to the classic builder on engines without BuildKit
// (e.g. Windows containers).
func (d *Docker) Build(ctx context.Context, dir, tag string, log io.Writer) error {
	if d.cli == nil {
		return ErrNoDocker
	}
	ping, err := d.cli.Ping(ctx, client.PingOptions{})
	if err != nil {
		return ErrNoDocker
	}
	if ping.BuilderVersion == build.BuilderBuildKit {
		return d.buildKit(ctx, dir, tag, log)
	}
	fmt.Fprintln(log, "engine has no BuildKit, using the classic builder")
	return d.buildClassic(ctx, dir, tag, log)
}

func (d *Docker) buildKit(ctx context.Context, dir, tag string, log io.Writer) error {
	bk, err := bkclient.New(ctx, "",
		bkclient.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return d.cli.DialHijack(ctx, "/grpc", "h2c", nil)
		}),
		bkclient.WithSessionDialer(func(ctx context.Context, proto string, meta map[string][]string) (net.Conn, error) {
			return d.cli.DialHijack(ctx, "/session", proto, meta)
		}),
	)
	if err != nil {
		return fmt.Errorf("connect to buildkit: %w", err)
	}
	defer bk.Close()

	src, err := fsutil.NewFS(dir)
	if err != nil {
		return err
	}
	status := make(chan *bkclient.SolveStatus)
	done := make(chan struct{})
	go func() {
		printProgress(status, log)
		close(done)
	}()
	_, err = bk.Solve(ctx, nil, bkclient.SolveOpt{
		Frontend:      "dockerfile.v0",
		FrontendAttrs: map[string]string{"filename": "Dockerfile"},
		LocalMounts:   map[string]fsutil.FS{"context": src, "dockerfile": src},
		Exports:       []bkclient.ExportEntry{{Type: "moby", Attrs: map[string]string{"name": tag}}},
	}, status)
	<-done
	return err
}

// printProgress writes BuildKit progress as plain lines: each step when it starts,
// its output, and how it finished.
func printProgress(ch <-chan *bkclient.SolveStatus, log io.Writer) {
	started := map[digest.Digest]bool{}
	finished := map[digest.Digest]bool{}
	for s := range ch {
		for _, v := range s.Vertexes {
			if v.Started != nil && !started[v.Digest] {
				started[v.Digest] = true
				fmt.Fprintln(log, v.Name)
			}
			if v.Completed != nil && !finished[v.Digest] {
				finished[v.Digest] = true
				switch {
				case v.Error != "":
					fmt.Fprintf(log, "ERROR %s: %s\n", v.Name, v.Error)
				case v.Cached:
					fmt.Fprintf(log, "CACHED %s\n", v.Name)
				}
			}
		}
		for _, l := range s.Logs {
			log.Write(l.Data)
		}
		for _, w := range s.Warnings {
			fmt.Fprintf(log, "WARNING %s\n", w.Short)
		}
	}
}

func (d *Docker) buildClassic(ctx context.Context, dir, tag string, log io.Writer) error {
	pr, pw := io.Pipe()
	go func() { pw.CloseWithError(archive.Tar(pw, dir)) }()
	res, err := d.cli.ImageBuild(ctx, pr, client.ImageBuildOptions{
		Tags:       []string{tag},
		Dockerfile: "Dockerfile",
		Remove:     true,
		Version:    build.BuilderV1,
	})
	if err != nil {
		pr.Close()
		return err
	}
	defer res.Body.Close()
	dec := json.NewDecoder(res.Body)
	for {
		var msg struct {
			Stream string `json:"stream"`
			Status string `json:"status"`
			Error  string `json:"error"`
		}
		if err := dec.Decode(&msg); err == io.EOF {
			return nil
		} else if err != nil {
			return err
		}
		if msg.Error != "" {
			return errors.New(msg.Error)
		}
		io.WriteString(log, msg.Stream)
		if msg.Status != "" {
			fmt.Fprintln(log, msg.Status)
		}
	}
}
