# Music Intent Coordinator

A centralized music intent coordinator for home automation that eliminates duplication by managing playlists and speaker mappings in a single SQLite database.

## Purpose

- **Eliminate Duplication**: Playlists and speaker IDs are no longer hardcoded across multiple automations
- **Centralize Configuration**: One database holds all mappings (intent → playlist, location → speaker entity)
- **Decouple Intent from Execution**: Home Assistant (with Music Assistant) becomes the executor, not the brain
- **Make Changes Low-Friction**: Change a playlist or speaker once in the DB → all automations automatically respect it

## Architecture

```
┌─────────────────┐
│  Home Assistant │
│   Automations   │
└────────┬────────┘
         │ MQTT / HTTP POST
         │ {intent: "christmas", location: "garage"}
         ▼
┌─────────────────┐
│   Coordinator   │
│   (Go Service)  │
└────────┬────────┘
         │
    ┌────┴────┐
    │         │
    ▼         ▼
┌────────┐ ┌──────────────┐
│ SQLite │ │ Home         │
│   DB   │ │ Assistant    │
│        │ │ (via MQTT)   │
└────────┘ └──────────────┘
```

## Components

### SQLite Database

- **intent**: Maps intent names to playlists (supports multiple playlists per intent with random selection)
  - `name` (TEXT, UNIQUE): Intent identifier (e.g., "christmas", "workout")
  - `playlist` (TEXT): Playlist URI(s) stored as JSON array
- **location**: Maps location names to speaker entities
  - `name` (TEXT, UNIQUE): Location identifier (e.g., "garage", "living_room")
  - `speaker_entity` (TEXT): Home Assistant media player entity ID
- **playlist_group**: Named groups of playlists for reuse across intents

### Coordinator Service (Go)

- **MQTT-based communication**: Listens for play requests on `music-coordinator/play` and publishes commands to Home Assistant via MQTT
- **HTTP API**: REST API for CRUD operations and a web UI for management
- **Stateless**: SQLite DB is the authoritative state

## Setup

### Prerequisites

- [mise](https://mise.jdx.dev/) (recommended) or Go 1.23+
- Home Assistant with Music Assistant (mass) integration
- An MQTT broker (e.g., Mosquitto)
- Home Assistant API token (for media player sync feature)

### Installation

```bash
git clone https://github.com/YOUR_USERNAME/music-coordinator.git
cd music-coordinator

# Using mise (recommended)
mise install
mise run build

# Or manually
go mod download
go build -o music-coordinator .
```

### Configuration

Set environment variables (or use a `.env` file with Docker):

```bash
export MQTT_BROKER="tcp://localhost:1883"        # Required
export MQTT_USER=""                               # Optional
export MQTT_PASS=""                               # Optional
export HA_URL="http://homeassistant.local:8123"   # For media player sync
export HA_API_TOKEN="your-long-lived-token"       # For media player sync
export PORT="8080"                                # Optional, defaults to 8080
export DB_PATH="./music_coordinator.db"           # Optional
```

### Run

```bash
# Using mise
mise run run

# Or directly
./music-coordinator
```

The database schema is automatically created on first run. To populate with example data:

```bash
sqlite3 music_coordinator.db < init_db.sql
```

## Web UI

Once running, open `http://localhost:8080/` in your browser to:

- **Manage intents** with support for multiple playlists and playlist groups
- **Manage locations** with speaker entity mapping
- **Sync locations** from Home Assistant media players automatically
- **Manage playlist groups** for reusable sets of playlists

## Usage

### MQTT Communication (Primary)

The coordinator listens for play requests via MQTT:

- **Listen topic**: `music-coordinator/play`
- **Publish topic**: `homeassistant/service/mass/play_media`

```json
{
  "intent": "christmas",
  "location": "garage"
}
```

### HTTP API

#### Play Music

**POST** `/api/play`

```json
{
  "intent": "christmas",
  "location": "garage"
}
```

#### CRUD Endpoints

| Resource | List | Get | Create | Update | Delete |
|----------|------|-----|--------|--------|--------|
| Intents | `GET /api/intents` | `GET /api/intents/{name}` | `POST /api/intents` | `PUT /api/intents/{name}` | `DELETE /api/intents/{name}` |
| Locations | `GET /api/locations` | `GET /api/locations/{name}` | `POST /api/locations` | `PUT /api/locations/{name}` | `DELETE /api/locations/{name}` |
| Playlist Groups | `GET /api/playlist-groups` | `GET /api/playlist-groups/{name}` | `POST /api/playlist-groups` | `PUT /api/playlist-groups/{name}` | `DELETE /api/playlist-groups/{name}` |

#### Other Endpoints

- `GET /api/media-players` -- List media players from Home Assistant
- `POST /api/sync-locations` -- Auto-create locations from Home Assistant media players
- `GET /api/available-playlists` -- List all known playlist URIs
- `GET /health` -- Health check

## Home Assistant Integration

### Using MQTT (Recommended)

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

### Using REST Command

Add to `configuration.yaml`:

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

See the `examples/` directory for more integration patterns.

## Docker Deployment

```bash
docker-compose up -d
```

Or build and run manually:

```bash
docker build -t music-coordinator .
docker run -d \
  -p 8080:8080 \
  -v $(pwd)/data:/data \
  -e MQTT_BROKER="tcp://your-mqtt-broker:1883" \
  -e HA_API_TOKEN="your-token" \
  -e HA_URL="http://homeassistant.local:8123" \
  music-coordinator
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | HTTP server port |
| `DB_PATH` | `./music_coordinator.db` | SQLite database file path |
| `MQTT_BROKER` | `tcp://localhost:1883` | MQTT broker URL |
| `MQTT_USER` | | MQTT username (optional) |
| `MQTT_PASS` | | MQTT password (optional) |
| `MQTT_CLIENT_ID` | `music-coordinator` | MQTT client ID |
| `HA_URL` | `http://homeassistant.local:8123` | Home Assistant URL (for media player sync) |
| `HA_API_TOKEN` | | Home Assistant long-lived access token (for media player sync) |
| `MA_API_URL` | `http://localhost:8097` | Music Assistant API URL (reserved for future use) |

## Development

This project uses [mise](https://mise.jdx.dev/) for tool management and [hk](https://hk.jdx.dev/) for git hooks.

```bash
# Install tools
mise install

# Install git hooks
hk install

# Available tasks
mise run build     # Build the binary
mise run run       # Build and run
mise run test      # Run tests
mise run fmt       # Format code
mise run lint      # Run go vet
mise run clean     # Remove build artifacts
```

## Troubleshooting

### "Intent not found" error
- Check that the intent exists: `sqlite3 music_coordinator.db "SELECT * FROM intent;"`
- Intent names are case-sensitive

### "Location not found" error
- Check that the location exists: `sqlite3 music_coordinator.db "SELECT * FROM location;"`
- Location names are case-sensitive

### MQTT connection issues
- Verify `MQTT_BROKER` is set correctly (include `tcp://` prefix)
- Check broker is reachable and credentials are correct

### Database locked errors
- Ensure only one instance of the coordinator is running
- Check file permissions on the database file

## License

MIT
