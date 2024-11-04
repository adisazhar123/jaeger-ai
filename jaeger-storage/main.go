package main

import (
	"context"
	"fmt"
	"github.com/jaegertracing/jaeger/plugin/storage/grpc/shared"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
	"github.com/jmoiron/sqlx"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/reflection"
	"jaeger-storage/storage"
	"log"
	"net"
	"os/signal"
	"sync"
	"syscall"
)

// adapted from https://github.com/jaegertracing/jaeger/blob/main/cmd/remote-storage/app/server.go
func createGrpcHandler(db *sqlx.DB, neo4jDriver *neo4j.DriverWithContext) (*shared.GRPCHandler, error) {
	spanWriter := NewWriterClient(storage.NewSqlWriter(db), storage.NewNeo4jWriter(neo4jDriver))
	spanReader := NewReaderDBClient(db)

	impl := &shared.GRPCHandlerStorageImpl{
		SpanReader: func() spanstore.Reader {
			return spanReader
		},
		SpanWriter: func() spanstore.Writer {
			return spanWriter
		},
		DependencyReader: func() dependencystore.Reader {
			return dependencyReaderStub{}
		},
		// these are okay to return nil because the grpc-handler has nil checks
		ArchiveSpanReader: func() spanstore.Reader {
			return nil
		},
		ArchiveSpanWriter: func() spanstore.Writer {
			return nil
		},
		StreamingSpanWriter: func() spanstore.Writer {
			return nil
		},
	}

	return shared.NewGRPCHandler(impl), nil
}

func createGrpcServer(handler *shared.GRPCHandler) (*grpc.Server, error) {
	server := grpc.NewServer()
	healthServer := health.NewServer()
	reflection.Register(server)
	if err := handler.Register(server, healthServer); err != nil {
		log.Println("[createGrpcServer][error] an error occurred while registering handlers", err)
		return nil, err
	}

	return server, nil
}

type GrpcServer struct {
	server      *grpc.Server
	grpcConn    net.Listener
	wg          sync.WaitGroup
	neo4jDriver neo4j.DriverWithContext
}

func (s *GrpcServer) Start() error {
	address := ":54321"
	listener, err := net.Listen("tcp", address)
	if err != nil {
		log.Println("[Start][error] cannot start listen", err)
		return err
	}
	log.Println(fmt.Sprintf("[Start] starting grpc server at address %s 🚀", address))
	s.grpcConn = listener
	s.wg.Add(1)
	go s.serve()

	return nil
}

func (s *GrpcServer) serve() {
	defer s.wg.Done()
	if err := s.server.Serve(s.grpcConn); err != nil {
		log.Fatalln("[serve][error] grpc server exited", err)
	}
}

func (s *GrpcServer) Close() error {
	s.server.Stop()
	s.grpcConn.Close()
	s.wg.Wait()
	s.neo4jDriver.Close(context.Background())
	return nil
}

func NewGrpcServer() (*GrpcServer, error) {
	db, err := NewDb(NewDbOpt{
		Username: "postgres",
		Password: "password",
		DbName:   "jaeger-storage",
	})
	if err != nil {
		log.Println("error connecting to DB", err)
		return nil, err
	}
	neo4jDriver, err := NewNeo4jDriver()
	if err != nil {
		log.Println("error connecting to neo4j", err)
		return nil, err
	}
	handler, err := createGrpcHandler(db, neo4jDriver)
	if err != nil {
		return nil, err
	}
	server, err := createGrpcServer(handler)
	if err != nil {
		return nil, err
	}

	return &GrpcServer{
		server:      server,
		neo4jDriver: *neo4jDriver,
	}, nil
}

func main() {
	server, err := NewGrpcServer()
	if err != nil {
		log.Fatalln("[main] cannot create new grpc server", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := server.Start(); err != nil {
		log.Fatalln("[main] cannot start grpc server", err)
	}

	<-ctx.Done()
	log.Println("[main] stopping grpc server, received a signal")
	if err := server.Close(); err != nil {
		log.Fatalln("[main] cannot close grpc server", err)
	}

	log.Println("[main] bye bye 👋")
}
