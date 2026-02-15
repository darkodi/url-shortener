# 1. Install PostgreSQL driver
go get github.com/lib/pq

# 2. Update code files (above)

# 3. Build and start
docker compose down -v  # Clean slate
docker compose build
docker compose up -d

# 4. Check replication status
docker exec postgres-primary psql -U urlshortener -d urlshortener -c "SELECT * FROM pg_stat_replication;"

# 5. Test the app
curl -X POST http://localhost:8080/shorten \
  -H "Content-Type: application/json" \
  -d '{"url": "https://www.meteoblue.com/"}'

