-- Initialize database with example data
-- Run this after the service creates the schema

-- Example intents (replace playlist URIs with your own)
INSERT OR IGNORE INTO intent (name, playlist) VALUES
    ('christmas', 'spotify:playlist:YOUR_CHRISTMAS_PLAYLIST_ID'),
    ('saturday_morning', 'spotify:playlist:YOUR_SATURDAY_PLAYLIST_ID'),
    ('sunday_morning', 'spotify:playlist:YOUR_SUNDAY_PLAYLIST_ID'),
    ('leaving_alert', 'spotify:playlist:YOUR_ALERT_PLAYLIST_ID');

-- Example locations
INSERT OR IGNORE INTO location (name, speaker_entity) VALUES
    ('garage', 'media_player.garage'),
    ('living_room', 'media_player.lounge'),
    ('bedroom', 'media_player.bed_room'),
    ('whole_house', 'media_player.whole_house'),
    ('downstairs', 'media_player.down_stairs'),
    ('upstairs', 'media_player.upstairs');

