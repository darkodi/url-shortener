package sharding

import (
	"crypto/md5"
	"database/sql"
	"encoding/binary"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/darkodi/url-shortener/internal/config"

	_ "github.com/lib/pq" // PostgreSQL driver
)

// ShardRouter manages database connections across multiple shards
type ShardRouter struct {
	config      *config.Config
	shards      []*Shard
	numShards   int
	initialized bool
	mu          sync.RWMutex
}

// Shard represents a single database shard with primary and replica connections
type Shard struct {
	ShardID      int
	Primary      *sql.DB
	Replicas     []*sql.DB
	replicaIndex uint32 // For round-robin replica selection
	replicaCount int
}

// NewShardRouter creates a new shard router from configuration
func NewShardRouter(cfg *config.Config) (*ShardRouter, error) {
	if cfg.NumShards == 0 {
		return nil, fmt.Errorf("number of shards must be greater than 0")
	}

	router := &ShardRouter{
		config:    cfg,
		numShards: cfg.NumShards,
		shards:    make([]*Shard, cfg.NumShards),
	}

	return router, nil
}

// Initialize establishes connections to all shard databases
func (r *ShardRouter) Initialize() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.initialized {
		return fmt.Errorf("shard router already initialized")
	}

	for i := 0; i < r.numShards; i++ {
		shardConfig := r.config.Shards[i]

		// Connect to primary
		primaryDSN := shardConfig.Primary.BuildPostgresConnectionString()
		primaryDB, err := sql.Open(shardConfig.Primary.Driver, primaryDSN)
		if err != nil {
			return fmt.Errorf("failed to open primary connection for shard %d: %w", i, err)
		}

		// Configure primary connection pool
		primaryDB.SetMaxOpenConns(shardConfig.Primary.MaxOpenConns)
		primaryDB.SetMaxIdleConns(shardConfig.Primary.MaxIdleConns)

		// Test primary connection
		if err := primaryDB.Ping(); err != nil {
			primaryDB.Close()
			return fmt.Errorf("failed to ping primary for shard %d: %w", i, err)
		}

		// Connect to replicas
		replicas := make([]*sql.DB, len(shardConfig.Replicas))
		for j, replicaConfig := range shardConfig.Replicas {
			replicaDSN := replicaConfig.BuildPostgresConnectionString()
			replicaDB, err := sql.Open(replicaConfig.Driver, replicaDSN)
			if err != nil {
				// Clean up already opened connections
				primaryDB.Close()
				for k := 0; k < j; k++ {
					replicas[k].Close()
				}
				return fmt.Errorf("failed to open replica %d connection for shard %d: %w", j, i, err)
			}

			// Configure replica connection pool
			replicaDB.SetMaxOpenConns(replicaConfig.MaxOpenConns)
			replicaDB.SetMaxIdleConns(replicaConfig.MaxIdleConns)

			// Test replica connection
			if err := replicaDB.Ping(); err != nil {
				replicaDB.Close()
				primaryDB.Close()
				for k := 0; k < j; k++ {
					replicas[k].Close()
				}
				return fmt.Errorf("failed to ping replica %d for shard %d: %w", j, i, err)
			}

			replicas[j] = replicaDB
		}

		r.shards[i] = &Shard{
			ShardID:      i,
			Primary:      primaryDB,
			Replicas:     replicas,
			replicaCount: len(replicas),
			replicaIndex: 0,
		}
	}

	r.initialized = true
	return nil
}

// GetShardForKey determines which shard a key belongs to using consistent hashing
func (r *ShardRouter) GetShardForKey(key string) int {
	// Use MD5 hash for consistent distribution
	hash := md5.Sum([]byte(key))

	// Convert first 8 bytes to uint64
	hashValue := binary.BigEndian.Uint64(hash[:8])

	// Modulo to get shard index
	shardID := int(hashValue % uint64(r.numShards))

	return shardID
}

// GetPrimaryDB returns the primary database connection for a given key (for writes)
func (r *ShardRouter) GetPrimaryDB(key string) (*sql.DB, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if !r.initialized {
		return nil, fmt.Errorf("shard router not initialized")
	}

	shardID := r.GetShardForKey(key)
	return r.shards[shardID].Primary, nil
}

// GetReplicaDB returns a replica database connection for a given key (for reads)
// Uses round-robin selection among replicas
func (r *ShardRouter) GetReplicaDB(key string) (*sql.DB, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if !r.initialized {
		return nil, fmt.Errorf("shard router not initialized")
	}

	shardID := r.GetShardForKey(key)
	shard := r.shards[shardID]

	if shard.replicaCount == 0 {
		// Fall back to primary if no replicas available
		return shard.Primary, nil
	}

	// Round-robin replica selection using atomic operations
	index := atomic.AddUint32(&shard.replicaIndex, 1)
	replicaIdx := int(index) % shard.replicaCount

	return shard.Replicas[replicaIdx], nil
}

// GetPrimaryDBForShard returns the primary database connection for a specific shard ID
func (r *ShardRouter) GetPrimaryDBForShard(shardID int) (*sql.DB, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if !r.initialized {
		return nil, fmt.Errorf("shard router not initialized")
	}

	if shardID < 0 || shardID >= r.numShards {
		return nil, fmt.Errorf("invalid shard ID: %d (must be 0-%d)", shardID, r.numShards-1)
	}

	return r.shards[shardID].Primary, nil
}

// GetReplicaDBForShard returns a replica database connection for a specific shard ID
func (r *ShardRouter) GetReplicaDBForShard(shardID int) (*sql.DB, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if !r.initialized {
		return nil, fmt.Errorf("shard router not initialized")
	}

	if shardID < 0 || shardID >= r.numShards {
		return nil, fmt.Errorf("invalid shard ID: %d (must be 0-%d)", shardID, r.numShards-1)
	}

	shard := r.shards[shardID]

	if shard.replicaCount == 0 {
		// Fall back to primary if no replicas available
		return shard.Primary, nil
	}

	// Round-robin replica selection
	index := atomic.AddUint32(&shard.replicaIndex, 1)
	replicaIdx := int(index) % shard.replicaCount

	return shard.Replicas[replicaIdx], nil
}

// GetAllPrimaryDBs returns all primary database connections (useful for migrations)
func (r *ShardRouter) GetAllPrimaryDBs() ([]*sql.DB, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if !r.initialized {
		return nil, fmt.Errorf("shard router not initialized")
	}

	dbs := make([]*sql.DB, r.numShards)
	for i := 0; i < r.numShards; i++ {
		dbs[i] = r.shards[i].Primary
	}

	return dbs, nil
}

// Close closes all database connections
func (r *ShardRouter) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.initialized {
		return nil
	}

	var lastErr error
	for i, shard := range r.shards {
		// Close primary
		if err := shard.Primary.Close(); err != nil {
			lastErr = fmt.Errorf("failed to close primary for shard %d: %w", i, err)
		}

		// Close replicas
		for j, replica := range shard.Replicas {
			if err := replica.Close(); err != nil {
				lastErr = fmt.Errorf("failed to close replica %d for shard %d: %w", j, i, err)
			}
		}
	}

	r.initialized = false
	return lastErr
}

// GetNumShards returns the total number of shards
func (r *ShardRouter) GetNumShards() int {
	return r.numShards
}

// IsInitialized returns whether the router has been initialized
func (r *ShardRouter) IsInitialized() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.initialized
}

// HealthCheck checks the health of all shard connections
func (r *ShardRouter) HealthCheck() map[int]ShardHealth {
	r.mu.RLock()
	defer r.mu.RUnlock()

	health := make(map[int]ShardHealth)

	for i, shard := range r.shards {
		shardHealth := ShardHealth{
			ShardID:       i,
			PrimaryHealth: r.checkConnection(shard.Primary),
			ReplicaHealth: make([]bool, len(shard.Replicas)),
		}

		for j, replica := range shard.Replicas {
			shardHealth.ReplicaHealth[j] = r.checkConnection(replica)
		}

		health[i] = shardHealth
	}

	return health
}

// ShardHealth represents the health status of a shard
type ShardHealth struct {
	ShardID       int
	PrimaryHealth bool
	ReplicaHealth []bool
}

// checkConnection checks if a database connection is healthy
func (r *ShardRouter) checkConnection(db *sql.DB) bool {
	if db == nil {
		return false
	}
	return db.Ping() == nil
}
