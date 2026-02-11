# Docker Test Environment Guide

## Overview
This Docker setup provides an isolated test environment for Senechal Gateway, allowing you to test development changes without affecting production systems.

## Quick Start

### 1. Build and Start the Test Environment
```bash
cd ~/senechal-gw
docker compose up --build
```

### 2. Run in Background (Detached Mode)
```bash
docker compose up -d --build
```

### 3. View Logs
```bash
docker compose logs -f
```

### 4. Stop the Test Environment
```bash
docker compose down
```

### 5. Stop and Clean Up (including volumes)
```bash
docker compose down -v
```

## Testing the API

### Check Health
```bash
curl http://localhost:8080/health
```

### Trigger Echo Plugin Manually
```bash
curl -X POST http://localhost:8080/trigger/echo/poll \
  -H "Authorization: Bearer test_admin_token_change_me_in_production" \
  -H "Content-Type: application/json" \
  -d '{"message": "Test message from API"}'
```

### List Plugins (Read-Only Token)
```bash
curl http://localhost:8080/plugins \
  -H "Authorization: Bearer test_readonly_token_change_me"
```

## Directory Structure

```
~/senechal-gw/
├── Dockerfile              # Container build instructions
├── docker-compose.yml      # Test environment orchestration
├── config.test.yaml        # Test-specific configuration
├── .env.test              # Test environment variables
├── plugins/               # Plugin directory (mounted as volume)
├── data/                  # Persistent data (database)
└── logs/                  # Log output directory
```

## Key Differences from Production

1. **Configuration**: Uses `config.test.yaml` instead of `config.yaml`
2. **Database**: Separate database file `senechal-test.db` in `./data/`
3. **API**: Enabled by default for testing
4. **Logging**: Set to DEBUG level for detailed output
5. **Faster Schedules**: Plugins run more frequently for quicker feedback
6. **Tokens**: Test tokens (DO NOT use in production)

## Rebuilding After Code Changes

When the dev team makes changes to the Go code:

```bash
# Stop the current container
docker compose down

# Rebuild and start with new code
docker compose up --build
```

## Accessing the Container

To debug or inspect the running container:

```bash
docker compose exec senechal-gw /bin/bash
```

## Resetting Test Environment

To start fresh (removes all state):

```bash
# Stop and remove everything including volumes
docker compose down -v

# Start fresh
docker compose up --build
```

## Monitoring

### View Real-time Logs
```bash
docker compose logs -f senechal-gw
```

### Check Container Status
```bash
docker compose ps
```

### View Container Resource Usage
```bash
docker stats senechal-gw-test
```

## Testing New Plugins

1. Add your plugin to the `plugins/` directory
2. Update `config.test.yaml` to enable and configure it
3. Restart the container:
   ```bash
   docker compose restart
   ```

## Troubleshooting

### Container won't start
- Check logs: `docker compose logs`
- Verify config.test.yaml syntax
- Ensure .env.test has required variables

### Permission issues
- The container runs as non-root user `senechal`
- Check file permissions in mounted volumes

### Port already in use
- Change port in docker-compose.yml:
  ```yaml
  ports:
    - "8081:8080"  # Use 8081 instead
  ```

### Database locked
- Stop all containers: `docker compose down`
- Remove volume: `docker volume rm senechal-gw_senechal-data`

## CI/CD Integration

This setup can be used in CI/CD pipelines:

```bash
# Run tests
docker compose up -d
sleep 10  # Wait for startup
# Run your test scripts here
docker compose down -v
```

## What to Test

When the dev team delivers new code:

1. **Build Success**: Does it compile and build in Docker?
2. **Startup**: Does the service start without errors?
3. **API Endpoints**: Do the API endpoints respond correctly?
4. **Plugin Execution**: Do plugins execute as scheduled?
5. **State Persistence**: Is state saved and restored correctly?
6. **Error Handling**: How does it handle plugin failures?
7. **Configuration**: Does config reload work (if supported)?
8. **Performance**: Check resource usage and response times

## Test Workflow Example

```bash
# 1. Pull latest code
cd ~/senechal-gw
git pull origin main

# 2. Build and start test environment
docker compose down -v  # Clean slate
docker compose up --build -d

# 3. Wait for startup
sleep 10

# 4. Test API health
curl http://localhost:8080/health

# 5. Test manual plugin trigger
curl -X POST http://localhost:8080/trigger/echo/poll \
  -H "Authorization: Bearer test_admin_token_change_me_in_production" \
  -H "Content-Type: application/json"

# 6. Monitor logs
docker compose logs -f

# 7. When done testing
docker compose down -v
```
