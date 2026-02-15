#!/bin/bash
set -e

echo "🔧 Configuring PostgreSQL for replication..."

# Configure pg_hba.conf to allow replication connections
cat >> "$PGDATA/pg_hba.conf" <<EOF

# TYPE  DATABASE        USER            ADDRESS                 METHOD
# Allow replication connections from Docker network
host    replication     urlshortener    172.0.0.0/8             trust
host    all             urlshortener    172.0.0.0/8             trust
EOF

echo "✅ Replication configuration complete!"
