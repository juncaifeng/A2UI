package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	pb "github.com/juncaifeng/a2ui-agent/gen/go/a2ui/agent/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
 grpcAddr := flag.String("grpc-addr", "localhost:50051", "Python agent gRPC server address")
 httpAddr := flag.String("http-addr", "localhost:8081", "Gateway HTTP listen address")
 flag.Parse()

 ctx := context.Background()

 conn, err := grpc.NewClient(*grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
 if err != nil {
  log.Fatalf("Failed to dial gRPC server: %v", err)
 }
 defer conn.Close()

 mux := runtime.NewServeMux()
 if err := pb.RegisterAgentServiceHandler(ctx, mux, conn); err != nil {
  log.Fatalf("Failed to register gateway handler: %v", err)
 }

 log.Printf("A2UI Agent Gateway: REST → gRPC (%s)", *grpcAddr)
 log.Printf("Listening on %s", *httpAddr)
 fmt.Println()
 fmt.Println("Endpoints:")
 fmt.Printf("  POST   %s/v1/chat          → AgentService.Chat\n", *httpAddr)
 fmt.Printf("  POST   %s/v1/chat/stream   → AgentService.ChatStream\n", *httpAddr)
 fmt.Printf("  GET    %s/v1/models        → AgentService.ListModels\n", *httpAddr)
 fmt.Printf("  GET    %s/v1/sessions/{id} → AgentService.GetSession\n", *httpAddr)
 fmt.Printf("  DELETE %s/v1/sessions/{id} → AgentService.DeleteSession\n", *httpAddr)

 if err := http.ListenAndServe(*httpAddr, mux); err != nil {
  log.Fatalf("Gateway failed: %v", err)
 }
}
