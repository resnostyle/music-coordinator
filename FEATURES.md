# Features

## Multiple Playlists per Intent (Random Selection)

The coordinator supports multiple playlists per intent with automatic random selection. This is useful when you want variety in your music selection.

### How It Works

1. **Define Multiple Playlists**: When creating or editing an intent, you can enter multiple playlists (one per line in the web UI)
2. **Random Selection**: Each time the intent is triggered, the coordinator randomly selects one playlist from the list
3. **Backward Compatible**: Single playlists still work -- the system automatically detects the format

### Example Use Cases

- **Christmas Music**: Have multiple Christmas playlists and randomly pick one each time
- **Workout Music**: Different workout playlists for variety
- **Morning Music**: Multiple morning playlists to keep things fresh

### Playlist Groups

Playlist groups let you define a named set of playlists that can be shared across multiple intents:

1. Create a group (e.g., "holiday_mix") with several playlists
2. Assign the group to one or more intents
3. When triggered, a random playlist from the group is selected

### Database Format

Playlists are stored as a JSON array in the database:
```json
["spotify:playlist:PLAYLIST_ID_1", "spotify:playlist:PLAYLIST_ID_2", "spotify:playlist:PLAYLIST_ID_3"]
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
    "spotify:playlist:PLAYLIST_ID_1",
    "spotify:playlist:PLAYLIST_ID_2",
    "spotify:playlist:PLAYLIST_ID_3"
  ]
}
```

Or use a playlist group:
```json
{
  "name": "christmas",
  "playlist_group": "holiday_mix"
}
```

Or use the legacy single playlist format (still supported):
```json
{
  "name": "christmas",
  "playlist": "spotify:playlist:PLAYLIST_ID_1"
}
```

### Web UI

In the web UI:
1. Enter the intent name
2. Choose "Direct Playlists" or "Playlist Group" as the source
3. For direct playlists, enter one playlist URI per line
4. Click "Save Intent"
5. Each time this intent is used, a random playlist will be selected

### Logging

The coordinator logs which playlist was randomly selected:
```
[DB] Found 3 playlists for intent 'christmas', randomly selected: spotify:playlist:PLAYLIST_ID_2 (index 1)
```
