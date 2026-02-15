# Quick Start Guide

## Prerequisites

1. [mise](https://mise.jdx.dev/) (recommended) or **Go 1.23+** installed
2. **Home Assistant** with Music Assistant (mass) integration configured
3. **MQTT broker** (e.g., Mosquitto) accessible to both Home Assistant and this service
4. **Home Assistant API Token** (optional, for media player sync) -- Create at: Profile → Long-lived access tokens → Create Token

## Setup Steps

### 1. Install Dependencies

```bash
cd music-coordinator

# Using mise (recommended)
mise install

# Or manually
go mod download
```

### 2. Set Environment Variables

```bash
export MQTT_BROKER="tcp://localhost:1883"
export HA_API_TOKEN="your-long-lived-access-token-here"  # Optional, for media player sync
export HA_URL="http://homeassistant.local:8123"           # Optional, for media player sync
export PORT="8080"                                        # Optional, defaults to 8080
```

### 3. Build and Run

```bash
# Using mise
mise run build
mise run run

# Or manually
go build -o music-coordinator .
./music-coordinator

# Or use Make
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

You'll see a web interface where you can:
- View all intents and locations
- Add new intents and locations
- Edit existing entries
- Delete entries
- Sync locations from Home Assistant

### 7. Configure Home Assistant

Using MQTT (recommended), add an automation:

```yaml
automation:
  - alias: "Play Christmas Music"
    trigger:
      - platform: state
        entity_id: input_boolean.christmas_mode
        to: 'on'
    action:
      - service: mqtt.publish
        data:
          topic: "music-coordinator/play"
          payload: |
            {
              "intent": "christmas",
              "location": "garage"
            }
```

Or using REST command, add to `configuration.yaml`:

```yaml
rest_command:
  play_music_intent:
    url: "http://music-coordinator:8080/api/play"
    method: POST
    headers:
      Content-Type: "application/json"
    payload: |
      {
        "intent": "{{ intent }}",
        "location": "{{ location }}"
      }
```

Then use in automations (see `examples/` for more patterns):

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

## Docker Deployment

```bash
# Using docker-compose
export HA_API_TOKEN="your-token"
docker-compose up -d

# Or manually
docker build -t music-coordinator .
docker run -d \
  -p 8080:8080 \
  -v $(pwd)/data:/data \
  -e MQTT_BROKER="tcp://your-mqtt-broker:1883" \
  -e HA_API_TOKEN="your-token" \
  -e HA_URL="http://homeassistant.local:8123" \
  music-coordinator
```

## Troubleshooting

### "Intent not found" error
- Check database: `sqlite3 music_coordinator.db "SELECT * FROM intent;"`
- Ensure intent name matches exactly (case-sensitive)

### "Location not found" error
- Check database: `sqlite3 music_coordinator.db "SELECT * FROM location;"`
- Ensure location name matches exactly (case-sensitive)

### MQTT connection issues
- Verify `MQTT_BROKER` is set correctly (include `tcp://` prefix)
- Check broker is reachable and credentials are correct
- Check coordinator logs for connection errors

### Service won't start
- Check port 8080 is not in use: `lsof -i :8080`
- Verify Go is installed: `go version` (or use `mise install`)
- Check environment variables are set
