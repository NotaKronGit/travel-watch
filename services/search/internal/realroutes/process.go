package realroutes

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os/exec"

	"github.com/NotaKronGit/travel-watch/api/transport"
)

// Process uses local framed JSON, not a production network API. One owner calls it sequentially.
type Process struct {
	command *exec.Cmd
	input   io.WriteCloser
	output  *bufio.Scanner
	cancel  context.CancelFunc
}

func Start(ctx context.Context, path string) (*Process, error) {
	ctx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(ctx, path, "transport-stdio") // #nosec G204 -- executable is operator configuration, never model or user route input.
	cmd.Stderr = io.Discard
	in, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	if err = cmd.Start(); err != nil {
		cancel()
		return nil, errors.New("cannot start Collector command")
	}
	scan := bufio.NewScanner(out)
	scan.Buffer(make([]byte, 4096), 4<<20)
	return &Process{command: cmd, input: in, output: scan, cancel: cancel}, nil
}
func (p *Process) Close() { _ = p.input.Close(); p.cancel(); _ = p.command.Wait() }
func (p *Process) Call(ctx context.Context, q transport.Request) (transport.Response, error) {
	if err := ctx.Err(); err != nil {
		return transport.Response{}, err
	}
	stop := context.AfterFunc(ctx, p.cancel)
	defer stop()
	q.Version = 1
	if json.NewEncoder(p.input).Encode(q) != nil {
		return transport.Response{}, errors.New("Collector input unavailable")
	}
	if !p.output.Scan() {
		return transport.Response{}, errors.New("Collector output unavailable")
	}
	var r transport.Response
	if json.Unmarshal(p.output.Bytes(), &r) != nil || r.Version != 1 {
		return r, errors.New("invalid Collector reply")
	}
	if r.Error != "" {
		return r, errors.New(r.Error)
	}
	return r, ctx.Err()
}
