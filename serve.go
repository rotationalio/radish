package radish

import (
	"context"
	"fmt"
	"net"

	"github.com/kansaslabs/radish/api"
	"github.com/kansaslabs/x/out"
	"google.golang.org/grpc"
)

// Listen on the configured address and port for API requests and run prometheus metrics server.
func (r *Radish) Listen() (err error) {
	if !r.config.SuppressMetrics {
		if err = registerMetrics(); err != nil {
			return fmt.Errorf("could not register prometheus metrics: %s", err)
		}
		go serveMetrics(r.config.MetricsAddr)
	}

	// Open TCP socket to listen on from the configuration
	var sock net.Listener
	if sock, err = net.Listen("tcp", r.config.Addr); err != nil {
		return Errorf(ErrBadGateway, "could not listen on %s: %s", r.config.Addr, err)
	}
	defer sock.Close()
	out.Status("listening for requests on %s", r.config.Addr)

	// Initialize and run the gRPC server
	srv := grpc.NewServer()
	api.RegisterRadishServer(srv, r)
	return srv.Serve(sock)
}

// Shutdown the queue gracefully, stopping the server, completing any tasks in flight
// and stopping workers. Tasks cannot be delayed after shutdown is called.
func (r *Radish) Shutdown() (err error) {
	return Errorf(ErrUnknown, "shutdown is not implemented yet")
}

// Queue an asynchronous task from a gRPC request.
func (r *Radish) Queue(ctx context.Context, in *api.QueueRequest) (rep *api.QueueReply, err error) {
	rep = &api.QueueReply{Success: true}
	if rep.Uuid, err = r.Delay(in.Task, in.Params, in.Success, in.Failure); err != nil {
		rep.Success = false

		var ok bool
		if rep.Error, ok = err.(*api.Error); !ok {
			return nil, fmt.Errorf("could not cast error to API error: %s", err)
		}
	}

	return rep, nil
}

// Scale the number of workers on the server.
func (r *Radish) Scale(ctx context.Context, in *api.ScaleRequest) (rep *api.ScaleReply, err error) {
	rep = &api.ScaleReply{Success: true}
	if err = r.SetWorkers(int(in.Workers)); err != nil {
		rep.Success = false

		var ok bool
		if rep.Error, ok = err.(*api.Error); !ok {
			return nil, fmt.Errorf("could not cast error to API error: %s", err)
		}
		return rep, nil
	}

	rep.Workers = int32(r.NumWorkers())
	return rep, nil
}

// Status returns information about the state of the radish task queue.
func (r *Radish) Status(ctx context.Context, in *api.StatusRequest) (rep *api.StatusReply, err error) {
	rep = &api.StatusReply{
		Workers: int32(r.NumWorkers()),
		Queue:   uint64(len(r.tasks)),
		Tasks:   make([]string, 0, len(r.handlers)),
	}

	for name := range r.handlers {
		rep.Tasks = append(rep.Tasks, name)
	}

	return rep, nil
}
