package database

import (
	"context"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

var (
	Cache        *redis.Client
	CacheChannel chan string
)

// SetupRedis initializes the Redis client.
func SetupRedis() {
	Cache = redis.NewClient(&redis.Options{
		Addr: "redis:6379", // Redis server address
		DB:   0,            // Default DB
	})

	// Ping Redis to verify the connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := Cache.Ping(ctx).Result(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}

	log.Println("Successfully connected to Redis")
}

// SetupCacheChannel initializes the cache clearing mechanism.
func SetupCacheChannel() {
	CacheChannel = make(chan string)

	go func(ch chan string) {
		for {
			time.Sleep(5 * time.Second) // Wait for 5 seconds before processing the next key
			key := <-ch

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			if err := Cache.Del(ctx, key).Err(); err != nil {
				log.Printf("Failed to clear cache for key %s: %v", key, err)
			} else {
				log.Printf("Cache cleared for key: %s", key)
			}
		}
	}(CacheChannel)
}

// ClearCache sends keys to the cache channel for clearing.
func ClearCache(keys ...string) {
	for _, key := range keys {
		CacheChannel <- key
	}
}

// CloseRedis closes the Redis connection.
func CloseRedis() {
	if Cache != nil {
		if err := Cache.Close(); err != nil {
			log.Printf("Failed to close Redis connection: %v", err)
		} else {
			log.Println("Redis connection closed successfully")
		}
	}
}
