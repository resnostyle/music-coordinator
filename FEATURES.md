# Features

## Multiple Playlists per Intent (Random Selection)

The coordinator supports multiple playlists per intent with automatic random selection. This is useful when you want variety in your music selection.

### How It Works

1. **Define Multiple Playlists**: When creating or editing an intent, you can enter multiple playlists (one per line in the textarea)
2. **Random Selection**: Each time the intent is triggered, the coordinator randomly selects one playlist from the group
3. **Backward Compatible**: Single playlists still work - the system automatically detects the format

### Example Use Cases

- **Christmas Music**: Have multiple Christmas playlists and randomly pick one each time
- **Workout Music**: Different workout playlists for variety
- **Morning Music**: Multiple morning playlists to keep things fresh

### Database Format

Playlists are stored as a JSON array in the database:
```json
["spotify:playlist:abc123", "spotify:playlist:def456", "spotify:playlist:ghi789"]
```

The system also supports:
- **Legacy format**: Single playlist string (automatically converted)
- **Comma-separated**: `"playlist1, playlist2, playlist3"` (automatically converted to array)

### API Format

When creating/updating an intent via API:

```json
{
  "name": "christmas",
  "playlists": [
    "spotify:playlist:37i9dQZF1DXdd3gw5QVjt9",
    "spotify:playlist:37i9dQZF1DX3Ogo9pFvBkY",
    "spotify:playlist:37i9dQZF1DXa1rZf8gLhyz"
  ]
}
```

Or use the legacy single playlist format (still supported):
```json
{
  "name": "christmas",
  "playlist": "spotify:playlist:37i9dQZF1DXdd3gw5QVjt9"
}
```

### Web UI

In the web UI:
1. Enter the intent name
2. In the "Playlists" textarea, enter one playlist per line:
   ```
   spotify:playlist:37i9dQZF1DXdd3gw5QVjt9
   spotify:playlist:37i9dQZF1DX3Ogo9pFvBkY
   spotify:playlist:37i9dQZF1DXa1rZf8gLhyz
   ```
3. Click "Add Intent"
4. Each time this intent is used, a random playlist will be selected

### Logging

The coordinator logs which playlist was randomly selected:
```
[DB] Found 3 playlists for intent 'christmas', randomly selected: spotify:playlist:37i9dQZF1DX3Ogo9pFvBkY (index 1)
```

