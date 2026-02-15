# Quick Start Guide

## Prerequisites

1. **Go 1.21+** installed
2. **Home Assistant** with Music Assistant (mass) integration configured
3. **Home Assistant API Token** - Create one at: Profile → Long-lived access tokens → Create Token

## Setup Steps

### 1. Install Dependencies

```bash
cd music-coordinator
go mod download
```

### 2. Set Environment Variables

```bash
export HA_API_TOKEN="your-long-lived-access-token-here"
export HA_URL="http://homeassistant.local:8123"  # Adjust to your HA URL
export PORT="8080"  # Optional, defaults to 8080
```

### 3. Build and Run

```bash
# Build
go build -o music-coordinator .

# Run
./music-coordinator
```

Or use Make:
```bash
make build
make run
```

### 4. Initialize Database with Example Data

```bash
sqlite3 music_coordinator.db < init_db.sql
```

Or the database will be created automatically on first run (empty).

### 5. Test the Service

```bash
# Health check
curl http://localhost:8080/health

# Test playing music
curl -X POST http://localhost:8080/api/play \
  -H "Content-Type: application/json" \
  -d '{"intent": "christmas", "location": "garage"}'
```

### 6. Access the Web UI

Open your browser and navigate to:
```
http://localhost:8080/
```

You'll see a simple web interface where you can:
- View all intents and locations
- Add new intents and locations
- Edit existing entries
- Delete entries

No need to manually edit the database anymore!

### 7. Configure Home Assistant

Add to `configuration.yaml`:

```yaml
rest_command:
  play_music_intent:
    url: "http://localhost:8080/play"  # Adjust if running on different host
    method: POST
    headers:
      Content-Type: "application/json"
    payload: |
      {
        "intent": "{{ intent }}",
        "location": "{{ location }}"
      }
```

Then create an automation (see `examples/homeassistant_automation.yaml`):

```yaml
automation:
  - alias: "Play Christmas Music"
    trigger:
      - platform: state
        entity_id: input_boolean.christmas_mode
        to: 'on'
    action:
      - service: rest_command.play_music_intent
        data:
          intent: "christmas"
          location: "garage"
```

## Database Management

### View all intents:
```bash
sqlite3 music_coordinator.db "SELECT * FROM intent;"
```

### View all locations:
```bash
sqlite3 music_coordinator.db "SELECT * FROM location;"
```

### Add new intent:
```bash
sqlite3 music_coordinator.db "INSERT INTO intent (name, playlist) VALUES ('workout', 'spotify:playlist:YOUR_ID');"
```

### Add new location:
```bash
sqlite3 music_coordinator.db "INSERT INTO location (name, speaker_entity) VALUES ('office', 'media_player.office');"
```

## Docker Deployment

```bash
# Build
docker build -t music-coordinator .

# Run
docker run -d \
  -p 8080:8080 \
  -v $(pwd)/data:/data \
  -e HA_API_TOKEN="your-token" \
  -e HA_URL="http://homeassistant.local:8123" \
  music-coordinator
```

Or use docker-compose:
```bash
# Set HA_API_TOKEN in .env or export it
export HA_API_TOKEN="your-token"
docker-compose up -d
```

## Troubleshooting

### "Intent not found" error
- Check database: `sqlite3 music_coordinator.db "SELECT * FROM intent;"`
- Ensure intent name matches exactly (case-sensitive)

### "Location not found" error
- Check database: `sqlite3 music_coordinator.db "SELECT * FROM location;"`
- Ensure location name matches exactly (case-sensitive)

### "Failed to play music" error
- Verify HA_API_TOKEN is correct
- Check speaker entity exists in Home Assistant
- Verify Music Assistant integration is configured
- Check Home Assistant logs

### Service won't start
- Check port 8080 is not in use: `lsof -i :8080`
- Verify Go is installed: `go version`
- Check environment variables are set: `echo $HA_API_TOKEN`

