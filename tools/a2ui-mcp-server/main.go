package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/juncaifeng/a2ui-mcp-server/internal/a2ui"
	"github.com/juncaifeng/a2ui-mcp-server/internal/catalog"
	"github.com/juncaifeng/a2ui-mcp-server/internal/handler"
	"github.com/juncaifeng/a2ui-mcp-server/internal/session"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	addr := flag.String("addr", "localhost:8080", "listen address")
	specDir := flag.String("spec", defaultSpecDir(), "path to specification/v0_9/json directory")
	flag.Parse()

	// Load all catalogs
	catalogs, err := catalog.LoadAll(*specDir)
	if err != nil {
		log.Fatalf("Failed to load catalogs: %v", err)
	}
	for _, cat := range catalogs {
		log.Printf("Catalog %q: %d components", cat.CatalogID, len(cat.Components))
	}
	cat := catalog.MergeCatalogs(catalogs)
	log.Printf("Merged %d catalogs: %d total components", len(catalogs), len(cat.Components))

	store := session.NewStore()
	builder := a2ui.NewBuilder()

	// Create MCP server
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "a2ui-mcp-server",
		Version: "0.1.0",
	}, nil)

	// Register protocol tools (create_surface, update_data_model, delete_surface, render_ui)
	handler.RegisterProtocolTools(server, store, builder)
	handler.RegisterRenderTools(server, store, builder)

	// Register component tools from catalog
	if err := handler.RegisterComponentTools(server, cat, store); err != nil {
		log.Fatalf("Failed to register component tools: %v", err)
	}

	// Start StreamableHTTP server
	httpHandler := mcp.NewStreamableHTTPHandler(func(req *http.Request) *mcp.Server {
		return server
	}, nil)

	// Wrap with logging middleware
	logged := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sid := r.Header.Get("Mcp-Session-Id")
		log.Printf("← %s %s session=%s content-type=%s", r.Method, r.URL.Path, sid, r.Header.Get("Content-Type"))
		w2 := &loggingWriter{ResponseWriter: w}
		httpHandler.ServeHTTP(w2, r)
		log.Printf("→ %s %s status=%d", r.Method, r.URL.Path, w2.status)
	})

	log.Printf("A2UI MCP Server listening on %s", *addr)
	if err := http.ListenAndServe(*addr, logged); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func defaultSpecDir() string {
	// Default: look for specification/v0_9/json relative to the repo root
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	// Walk up from executable to find specification dir
	dir := filepath.Dir(exe)
	for i := 0; i < 10; i++ {
		candidate := filepath.Join(dir, "specification", "v0_9", "json")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		candidate = filepath.Join(dir, "spec", "v0_9", "json")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

// loggingWriter tracks the HTTP status code.
type loggingWriter struct {
	http.ResponseWriter
	status int
}

func (w *loggingWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *loggingWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func init() {
	// Ensure specDir flag has a reasonable default message
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: a2ui-mcp-server [options]\n\nOptions:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nIf -spec is not set, searches upward from the executable for specification/v0_9/json.\n")
	}
}
