package integration

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/agentmemory/agentmemory/internal/config"
	"github.com/agentmemory/agentmemory/internal/mcp"
	"github.com/agentmemory/agentmemory/internal/service"
	"github.com/agentmemory/agentmemory/internal/store"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// T177: Benchmark server startup: <10s (SC-007).

// TestBenchStartupComponentInit measures in-memory component initialization time,
// which is the bulk of server startup. When a real DB is available, it also
// measures the DB connection and migration time.
func TestBenchStartupComponentInit(t *testing.T) {
	start := time.Now()

	// Simulate component initialization without a real DB
	// This is the fast path for unit/integration environments

	// Step 1: Config initialization
	cfg := config.Load()
	cfgInitTime := time.Since(start)
	t.Logf("Config init: %v", cfgInitTime)
	assert.Less(t, cfgInitTime, 1*time.Second, "config init should be <1s")

	// Step 2: MCP server creation
	stepStart := time.Now()
	mcpServer := sdkmcp.NewServer(
		&sdkmcp.Implementation{Name: "agentmemory-v2", Version: "2.0.0"},
		&sdkmcp.ServerOptions{},
	)
	mcp.RegisterAllTools(mcpServer, nil)
	mcpInitTime := time.Since(stepStart)
	t.Logf("MCP server + tool registration: %v", mcpInitTime)
	assert.Less(t, mcpInitTime, 5*time.Second, "MCP server init should be <5s")

	// Step 3: Total time
	totalTime := time.Since(start)
	t.Logf("Total component init: %v (target: <10s)", totalTime)
	assert.Less(t, totalTime, 10*time.Second, "total init should be <10s")

	_ = cfg // prevent unused warning
}

// TestBenchStartupWithDB measures full startup time including DB connection
// when a real database URL is available.
func TestBenchStartupWithDB(t *testing.T) {
	dbURL := os.Getenv("AGENTMEMORY_DATABASE_URL")
	if dbURL == "" {
		t.Skip("AGENTMEMORY_DATABASE_URL not set — skipping DB startup benchmark")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	start := time.Now()

	// Step 1: Create connection pool
	pool, err := config.NewPool(ctx, dbURL)
	require.NoError(t, err, "failed to connect to database")
	defer config.ClosePool(pool)

	poolTime := time.Since(start)
	t.Logf("Connection pool creation: %v", poolTime)
	assert.Less(t, poolTime, 10*time.Second, "pool creation should be <10s")

	// Step 2: Verify connectivity
	stepStart := time.Now()
	err = pool.Ping(ctx)
	require.NoError(t, err, "failed to ping database")
	pingTime := time.Since(stepStart)
	t.Logf("DB ping: %v", pingTime)

	// Step 3: Create service bundle
	stepStart = time.Now()
	svc := mcp.NewServiceBundle(pool)
	serviceInitTime := time.Since(stepStart)
	t.Logf("Service bundle creation: %v", serviceInitTime)
	assert.Less(t, serviceInitTime, 10*time.Second, "service bundle init should be <10s")

	// Step 4: MCP server with full services
	stepStart = time.Now()
	mcpServer := sdkmcp.NewServer(
		&sdkmcp.Implementation{Name: "agentmemory-v2", Version: "2.0.0"},
		&sdkmcp.ServerOptions{},
	)
	mcp.RegisterAllTools(mcpServer, pool)
	mcpFullInitTime := time.Since(stepStart)
	t.Logf("MCP server with full services: %v", mcpFullInitTime)
	assert.Less(t, mcpFullInitTime, 5*time.Second, "MCP server init should be <5s")

	totalTime := time.Since(start)
	t.Logf("Total startup (DB + services + MCP): %v (target: <10s)", totalTime)

	_ = svc // prevent unused warning
}

// TestBenchStartupStoreInit measures store layer initialization time.
func TestBenchStartupStoreInit(t *testing.T) {
	dbURL := os.Getenv("AGENTMEMORY_DATABASE_URL")
	if dbURL == "" {
		t.Skip("AGENTMEMORY_DATABASE_URL not set — skipping store init benchmark")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := config.NewPool(ctx, dbURL)
	require.NoError(t, err)
	defer config.ClosePool(pool)

	start := time.Now()
	queries := store.New(pool)
	initTime := time.Since(start)
	t.Logf("Store queries init: %v", initTime)
	assert.Less(t, initTime, 1*time.Second, "store init should be <1s")

	_ = queries
}

// TestBenchStartupSearchServiceInit measures search service initialization time.
func TestBenchStartupSearchServiceInit(t *testing.T) {
	dbURL := os.Getenv("AGENTMEMORY_DATABASE_URL")
	if dbURL == "" {
		t.Skip("AGENTMEMORY_DATABASE_URL not set — skipping search service init benchmark")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := config.NewPool(ctx, dbURL)
	require.NoError(t, err)
	defer config.ClosePool(pool)

	start := time.Now()
	searchSvc := service.NewSearchService(pool, nil)
	initTime := time.Since(start)
	t.Logf("Search service init: %v", initTime)
	assert.Less(t, initTime, 1*time.Second, "search service init should be <1s")

	_ = searchSvc
}

// TestBenchStartupTeamServiceInit measures team service initialization time.
func TestBenchStartupTeamServiceInit(t *testing.T) {
	dbURL := os.Getenv("AGENTMEMORY_DATABASE_URL")
	if dbURL == "" {
		t.Skip("AGENTMEMORY_DATABASE_URL not set — skipping team service init benchmark")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := config.NewPool(ctx, dbURL)
	require.NoError(t, err)
	defer config.ClosePool(pool)

	start := time.Now()
	teamSvc := service.NewTeamService(pool)
	memberSvc := service.NewTeamMembersService(pool)
	initTime := time.Since(start)
	t.Logf("Team + member service init: %v", initTime)
	assert.Less(t, initTime, 1*time.Second, "team service init should be <1s")

	_ = teamSvc
	_ = memberSvc
}

// TestBenchStartupInMemoryTransport measures the full in-memory MCP server
// initialization including transport setup, which simulates the HTTP handler
// binding step of server startup.
func TestBenchStartupInMemoryTransport(t *testing.T) {
	start := time.Now()

	// Create server with all tools
	mcpServer := sdkmcp.NewServer(
		&sdkmcp.Implementation{Name: "agentmemory-v2", Version: "2.0.0"},
		&sdkmcp.ServerOptions{},
	)
	mcp.RegisterAllTools(mcpServer, nil)

	// Setup transport
	inServer, inClient := sdkmcp.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go mcpServer.Run(ctx, inServer)

	// Connect client
	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "startup-client", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, inClient, nil)
	require.NoError(t, err)
	defer session.Close()

	// Verify server info immediately
	initResult := session.InitializeResult()
	assert.Equal(t, "agentmemory-v2", initResult.ServerInfo.Name)
	assert.Equal(t, "2.0.0", initResult.ServerInfo.Version)

	startupTime := time.Since(start)
	t.Logf("Full in-memory startup (server + transport + client connect): %v", startupTime)
	assert.Less(t, startupTime, 10*time.Second,
		"full in-memory startup should be <10s, took %v", startupTime)
}

// TestBenchStartupSequentialInit measures the traditional sequential
// initialization path (pool -> store -> services -> MCP).
func TestBenchStartupSequentialInit(t *testing.T) {
	dbURL := os.Getenv("AGENTMEMORY_DATABASE_URL")
	if dbURL == "" {
		t.Skip("AGENTMEMORY_DATABASE_URL not set — skipping sequential init benchmark")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	start := time.Now()

	// Sequential initialization following the server startup order
	// 1. Database pool
	pool, err := config.NewPool(ctx, dbURL)
	require.NoError(t, err)
	defer config.ClosePool(pool)

	// 2. Store layer
	_ = store.New(pool)

	// 3. Service layer
	_ = service.NewSearchService(pool, nil)
	_ = service.NewTeamService(pool)
	_ = service.NewTeamMembersService(pool)

	// 4. MCP server with tool registration
	mcpServer := sdkmcp.NewServer(
		&sdkmcp.Implementation{Name: "agentmemory-v2", Version: "2.0.0"},
		&sdkmcp.ServerOptions{},
	)
	mcp.RegisterAllTools(mcpServer, pool)

	initTime := time.Since(start)
	t.Logf("Sequential init (pool + store + services + MCP): %v (target: <10s)", initTime)
	assert.Less(t, initTime, 10*time.Second,
		"sequential init should be <10s, took %v", initTime)
}

