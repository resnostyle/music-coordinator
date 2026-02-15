-- Initialize database with example data
-- Run this after the service creates the schema

-- Example intents
INSERT OR IGNORE INTO intent (name, playlist) VALUES
    ('christmas', 'spotify:user:spotify:playlist:37i9dQZF1DXdd3gw5QVjt9'),
    ('saturday_morning', 'spotify:user:spotify:playlist:37i9dQZF1DX3Ogo9pFvBkY'),
    ('sunday_morning', 'spotify:user:klovespotify:playlist:2Od4HqROiraxI93i7Q4ZQ2'),
    ('leaving_alert', 'spotify:user:spotify:playlist:37i9dQZF1DWWi0hHcPHnic');

-- Example locations
INSERT OR IGNORE INTO location (name, speaker_entity) VALUES
    ('garage', 'media_player.garage'),
    ('living_room', 'media_player.lounge'),
    ('bedroom', 'media_player.bed_room'),
    ('whole_house', 'media_player.whole_house'),
    ('downstairs', 'media_player.down_stairs'),
    ('upstairs', 'media_player.upstairs');

