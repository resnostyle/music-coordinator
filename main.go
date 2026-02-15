package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	_ "github.com/mattn/go-sqlite3"
)

const (
	defaultPort         = "8082"
	defaultDBPath       = "./music_coordinator.db"
	defaultHAURL        = "https://ha.bryanwp.com"
	defaultHAToken      = ""
	defaultMAAPIURL     = "http://192.168.2.245:8097"
	defaultMQTTBroker   = "mqtt.p.bryanwp.com:1883"
	defaultMQTTUser     = ""
	defaultMQTTPass     = ""
	defaultMQTTClientID = "music-coordinator"
	mqttPlayTopic       = "music-coordinator/play"
	mqttHATopic         = "homeassistant/service/mass/play_media"
	mediaPlayerPrefix   = "media_player."
)

type Config struct {
	Port         string
	DBPath       string
	HAURL        string
	HAToken      string
	MAAPIURL     string
	MQTTBroker   string
	MQTTUser     string
	MQTTPass     string
	MQTTClientID string
}

type IntentRequest struct {
	Intent   string `json:"intent"`
	Location string `json:"location"`
}

type IntentResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

type Database struct {
	db *sql.DB
}

func NewDatabase(dbPath string) (*Database, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Enable foreign keys for CASCADE deletes to work
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	database := &Database{db: db}
	if err := database.InitSchema(); err != nil {
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	// Clean up orphaned playlist_group_item entries
	if err := database.CleanupOrphanedPlaylistItems(); err != nil {
		log.Printf("[DB] Warning: Failed to cleanup orphaned playlist items: %v", err)
	}

	return database, nil
}

func (d *Database) InitSchema() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS intent (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			playlist TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS location (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			speaker_entity TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS playlist_group (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS playlist_group_item (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			group_name TEXT NOT NULL,
			playlist TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (group_name) REFERENCES playlist_group(name) ON DELETE CASCADE,
			UNIQUE(group_name, playlist)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_intent_name ON intent(name)`,
		`CREATE INDEX IF NOT EXISTS idx_location_name ON location(name)`,
		`CREATE INDEX IF NOT EXISTS idx_playlist_group_name ON playlist_group(name)`,
		`CREATE INDEX IF NOT EXISTS idx_playlist_group_item_group ON playlist_group_item(group_name)`,
	}

	for _, query := range queries {
		if _, err := d.db.Exec(query); err != nil {
			return fmt.Errorf("failed to execute query: %w", err)
		}
	}

	return d.migrateSchema()
}

func (d *Database) migrateSchema() error {
	var count int
	err := d.db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('intent') WHERE name = 'playlist_group'`).Scan(&count)
	if err == nil && count > 0 {
		return nil
	}

	_, err = d.db.Exec(`ALTER TABLE intent ADD COLUMN playlist_group TEXT`)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate column") || strings.Contains(err.Error(), "already exists") {
			return nil
		}
		return fmt.Errorf("failed to add playlist_group column: %w", err)
	}
	return nil
}

// GetIntentPlaylist returns a randomly selected playlist from the intent's playlist group
func (d *Database) GetIntentPlaylist(intentName string) (string, error) {
	var playlistData string
	var playlistGroup sql.NullString
	err := d.db.QueryRow("SELECT playlist, playlist_group FROM intent WHERE name = ?", intentName).
		Scan(&playlistData, &playlistGroup)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("intent '%s' not found", intentName)
	}
	if err != nil {
		return "", fmt.Errorf("failed to query intent: %w", err)
	}

	// Check if using a playlist group
	if playlistGroup.Valid && playlistGroup.String != "" {
		playlists, err := d.GetGroupPlaylists(playlistGroup.String)
		if err != nil {
			return "", fmt.Errorf("failed to get group playlists: %w", err)
		}
		return selectRandomPlaylist(playlists)
	}

	// Parse and select from direct playlists
	playlists := parsePlaylists(playlistData)
	return selectRandomPlaylist(playlists)
}

func (d *Database) GetLocationSpeaker(locationName string) (string, error) {
	var speakerEntity string
	err := d.db.QueryRow("SELECT speaker_entity FROM location WHERE name = ?", locationName).Scan(&speakerEntity)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("location '%s' not found", locationName)
	}
	if err != nil {
		return "", fmt.Errorf("failed to query location: %w", err)
	}
	return speakerEntity, nil
}

// Intent CRUD methods
type Intent struct {
	ID            int      `json:"id"`
	Name          string   `json:"name"`
	Playlist      string   `json:"playlist"`       // For backward compatibility (single playlist)
	Playlists     []string `json:"playlists"`      // New format (multiple playlists)
	PlaylistGroup string   `json:"playlist_group"` // Reference to a playlist group
}

type PlaylistGroup struct {
	ID        int      `json:"id"`
	Name      string   `json:"name"`
	Playlists []string `json:"playlists"`
}

func (d *Database) GetAllIntents() ([]Intent, error) {
	rows, err := d.db.Query("SELECT id, name, playlist, playlist_group FROM intent ORDER BY name")
	if err != nil {
		return nil, fmt.Errorf("failed to query intents: %w", err)
	}
	defer rows.Close()

	var intents []Intent
	for rows.Next() {
		var intent Intent
		var playlistData string
		var playlistGroup sql.NullString
		if err := rows.Scan(&intent.ID, &intent.Name, &playlistData, &playlistGroup); err != nil {
			return nil, fmt.Errorf("failed to scan intent: %w", err)
		}

		if playlistGroup.Valid && playlistGroup.String != "" {
			intent.PlaylistGroup = playlistGroup.String
			groupPlaylists, _ := d.GetGroupPlaylists(playlistGroup.String)
			intent.Playlists = groupPlaylists
			if len(groupPlaylists) > 0 {
				intent.Playlist = groupPlaylists[0]
			}
		} else {
			playlists := parsePlaylists(playlistData)
			intent.Playlists = playlists
			if len(playlists) > 0 {
				intent.Playlist = playlists[0]
			}
		}
		intents = append(intents, intent)
	}
	return intents, nil
}

func (d *Database) GetIntent(name string) (*Intent, error) {
	var intent Intent
	var playlistData string
	var playlistGroup sql.NullString
	err := d.db.QueryRow("SELECT id, name, playlist, playlist_group FROM intent WHERE name = ?", name).
		Scan(&intent.ID, &intent.Name, &playlistData, &playlistGroup)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("intent '%s' not found", name)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query intent: %w", err)
	}

	if playlistGroup.Valid && playlistGroup.String != "" {
		intent.PlaylistGroup = playlistGroup.String
		groupPlaylists, _ := d.GetGroupPlaylists(playlistGroup.String)
		intent.Playlists = groupPlaylists
		if len(groupPlaylists) > 0 {
			intent.Playlist = groupPlaylists[0]
		}
	} else {
		playlists := parsePlaylists(playlistData)
		intent.Playlists = playlists
		if len(playlists) > 0 {
			intent.Playlist = playlists[0]
		}
	}
	return &intent, nil
}

func (d *Database) CreateIntent(name string, playlists []string, playlistGroup string) error {
	if playlistGroup != "" {
		_, err := d.db.Exec("INSERT INTO intent (name, playlist, playlist_group) VALUES (?, ?, ?)", name, "", playlistGroup)
		return err
	}
	if len(playlists) == 0 {
		return fmt.Errorf("at least one playlist is required when not using a group")
	}
	playlistData, err := json.Marshal(playlists)
	if err != nil {
		return fmt.Errorf("failed to marshal playlists: %w", err)
	}
	_, err = d.db.Exec("INSERT INTO intent (name, playlist) VALUES (?, ?)", name, string(playlistData))
	return err
}

func (d *Database) UpdateIntent(name string, playlists []string, playlistGroup string) error {
	if playlistGroup != "" {
		result, err := d.db.Exec("UPDATE intent SET playlist = ?, playlist_group = ?, updated_at = CURRENT_TIMESTAMP WHERE name = ?", "", playlistGroup, name)
		if err != nil {
			return fmt.Errorf("failed to update intent: %w", err)
		}
		if rowsAffected, _ := result.RowsAffected(); rowsAffected == 0 {
			return fmt.Errorf("intent '%s' not found", name)
		}
		return nil
	}
	if len(playlists) == 0 {
		return fmt.Errorf("at least one playlist is required when not using a group")
	}
	playlistData, err := json.Marshal(playlists)
	if err != nil {
		return fmt.Errorf("failed to marshal playlists: %w", err)
	}
	result, err := d.db.Exec("UPDATE intent SET playlist = ?, playlist_group = NULL, updated_at = CURRENT_TIMESTAMP WHERE name = ?", string(playlistData), name)
	if err != nil {
		return fmt.Errorf("failed to update intent: %w", err)
	}
	if rowsAffected, _ := result.RowsAffected(); rowsAffected == 0 {
		return fmt.Errorf("intent '%s' not found", name)
	}
	return nil
}

func (d *Database) DeleteIntent(name string) error {
	result, err := d.db.Exec("DELETE FROM intent WHERE name = ?", name)
	if err != nil {
		return fmt.Errorf("failed to delete intent: %w", err)
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("intent '%s' not found", name)
	}
	return nil
}

// Location CRUD methods
type Location struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	SpeakerEntity string `json:"speaker_entity"`
}

func (d *Database) GetAllLocations() ([]Location, error) {
	rows, err := d.db.Query("SELECT id, name, speaker_entity FROM location ORDER BY name")
	if err != nil {
		return nil, fmt.Errorf("failed to query locations: %w", err)
	}
	defer rows.Close()

	var locations []Location
	for rows.Next() {
		var location Location
		if err := rows.Scan(&location.ID, &location.Name, &location.SpeakerEntity); err != nil {
			return nil, fmt.Errorf("failed to scan location: %w", err)
		}
		locations = append(locations, location)
	}
	return locations, nil
}

func (d *Database) GetLocation(name string) (*Location, error) {
	var location Location
	err := d.db.QueryRow("SELECT id, name, speaker_entity FROM location WHERE name = ?", name).Scan(&location.ID, &location.Name, &location.SpeakerEntity)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("location '%s' not found", name)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query location: %w", err)
	}
	return &location, nil
}

func (d *Database) CreateLocation(name, speakerEntity string) error {
	_, err := d.db.Exec("INSERT INTO location (name, speaker_entity) VALUES (?, ?)", name, speakerEntity)
	if err != nil {
		return fmt.Errorf("failed to create location: %w", err)
	}
	return nil
}

func (d *Database) UpdateLocation(name, speakerEntity string) error {
	result, err := d.db.Exec("UPDATE location SET speaker_entity = ?, updated_at = CURRENT_TIMESTAMP WHERE name = ?", speakerEntity, name)
	if err != nil {
		return fmt.Errorf("failed to update location: %w", err)
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("location '%s' not found", name)
	}
	return nil
}

func (d *Database) DeleteLocation(name string) error {
	result, err := d.db.Exec("DELETE FROM location WHERE name = ?", name)
	if err != nil {
		return fmt.Errorf("failed to delete location: %w", err)
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("location '%s' not found", name)
	}
	return nil
}

// Playlist Group CRUD methods
func (d *Database) GetAllPlaylistGroups() ([]PlaylistGroup, error) {
	rows, err := d.db.Query("SELECT id, name FROM playlist_group ORDER BY name")
	if err != nil {
		return nil, fmt.Errorf("failed to query playlist groups: %w", err)
	}
	defer rows.Close()

	var groups []PlaylistGroup
	for rows.Next() {
		var group PlaylistGroup
		if err := rows.Scan(&group.ID, &group.Name); err != nil {
			return nil, fmt.Errorf("failed to scan playlist group: %w", err)
		}
		// Get playlists for this group
		playlists, _ := d.GetGroupPlaylists(group.Name)
		group.Playlists = playlists
		groups = append(groups, group)
	}
	return groups, nil
}

func (d *Database) GetGroupPlaylists(groupName string) ([]string, error) {
	rows, err := d.db.Query("SELECT playlist FROM playlist_group_item WHERE group_name = ? ORDER BY playlist", groupName)
	if err != nil {
		return nil, fmt.Errorf("failed to query group playlists: %w", err)
	}
	defer rows.Close()

	var playlists []string
	for rows.Next() {
		var playlist string
		if err := rows.Scan(&playlist); err != nil {
			return nil, fmt.Errorf("failed to scan playlist: %w", err)
		}
		playlists = append(playlists, playlist)
	}
	return playlists, nil
}

func (d *Database) CreatePlaylistGroup(name string, playlists []string) error {
	tx, err := d.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err = tx.Exec("INSERT INTO playlist_group (name) VALUES (?)", name); err != nil {
		return fmt.Errorf("failed to create playlist group: %w", err)
	}

	for _, playlist := range playlists {
		if playlist == "" {
			continue
		}
		if _, err = tx.Exec("INSERT INTO playlist_group_item (group_name, playlist) VALUES (?, ?)", name, playlist); err != nil {
			return fmt.Errorf("failed to add playlist to group: %w", err)
		}
	}

	return tx.Commit()
}

func (d *Database) UpdatePlaylistGroup(name string, playlists []string) error {
	tx, err := d.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err = tx.Exec("DELETE FROM playlist_group_item WHERE group_name = ?", name); err != nil {
		return fmt.Errorf("failed to delete existing playlists: %w", err)
	}

	for _, playlist := range playlists {
		if playlist == "" {
			continue
		}
		if _, err = tx.Exec("INSERT INTO playlist_group_item (group_name, playlist) VALUES (?, ?)", name, playlist); err != nil {
			return fmt.Errorf("failed to add playlist to group: %w", err)
		}
	}

	if _, err = tx.Exec("UPDATE playlist_group SET updated_at = CURRENT_TIMESTAMP WHERE name = ?", name); err != nil {
		return fmt.Errorf("failed to update group: %w", err)
	}

	return tx.Commit()
}

func (d *Database) DeletePlaylistGroup(name string) error {
	result, err := d.db.Exec("DELETE FROM playlist_group WHERE name = ?", name)
	if err != nil {
		return fmt.Errorf("failed to delete playlist group: %w", err)
	}
	if rowsAffected, _ := result.RowsAffected(); rowsAffected == 0 {
		return fmt.Errorf("playlist group '%s' not found", name)
	}
	return nil
}

func (d *Database) GetAllAvailablePlaylists() ([]string, error) {
	playlists := make(map[string]bool)

	rows, err := d.db.Query("SELECT playlist FROM intent WHERE playlist != '' AND playlist_group IS NULL")
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var playlistData string
			if err := rows.Scan(&playlistData); err == nil {
				for _, p := range parsePlaylists(playlistData) {
					playlists[p] = true
				}
			}
		}
	}

	rows2, err := d.db.Query("SELECT DISTINCT playlist FROM playlist_group_item")
	if err == nil {
		defer rows2.Close()
		for rows2.Next() {
			var playlist string
			if err := rows2.Scan(&playlist); err == nil && playlist != "" {
				playlists[playlist] = true
			}
		}
	}

	result := make([]string, 0, len(playlists))
	for p := range playlists {
		result = append(result, p)
	}
	sort.Strings(result)
	return result, nil
}

// CleanupOrphanedPlaylistItems removes playlist_group_item entries that reference non-existent groups
func (d *Database) CleanupOrphanedPlaylistItems() error {
	result, err := d.db.Exec(`
		DELETE FROM playlist_group_item 
		WHERE group_name NOT IN (SELECT name FROM playlist_group)
	`)
	if err != nil {
		return fmt.Errorf("failed to cleanup orphaned playlist items: %w", err)
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected > 0 {
		log.Printf("[DB] Cleaned up %d orphaned playlist_group_item entries", rowsAffected)
	}
	return nil
}

func (d *Database) Close() error {
	return d.db.Close()
}

// parsePlaylists parses playlist data from various formats (JSON array, comma-separated, or single)
func parsePlaylists(data string) []string {
	var playlists []string
	if err := json.Unmarshal([]byte(data), &playlists); err == nil && len(playlists) > 0 {
		return playlists
	}
	if strings.Contains(data, ",") {
		parts := strings.Split(data, ",")
		for _, p := range parts {
			if trimmed := strings.TrimSpace(p); trimmed != "" {
				playlists = append(playlists, trimmed)
			}
		}
		return playlists
	}
	if data != "" {
		return []string{data}
	}
	return []string{}
}

// selectRandomPlaylist returns a random playlist from the list
func selectRandomPlaylist(playlists []string) (string, error) {
	if len(playlists) == 0 {
		return "", fmt.Errorf("no playlists available")
	}
	return playlists[rand.Intn(len(playlists))], nil
}

// setCORSHeaders sets common CORS headers
func setCORSHeaders(w http.ResponseWriter, methods ...string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	if len(methods) > 0 {
		w.Header().Set("Access-Control-Allow-Methods", strings.Join(methods, ", "))
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	}
}

// handleOptions handles OPTIONS requests for CORS
func handleOptions(w http.ResponseWriter) {
	w.WriteHeader(http.StatusOK)
}

type Coordinator struct {
	db         *Database
	config     *Config
	haClient   *HAClient
	mqttClient mqtt.Client
}

func NewCoordinator(db *Database, config *Config) (*Coordinator, error) {
	coordinator := &Coordinator{
		db:       db,
		config:   config,
		haClient: NewHAClient(config.HAURL, config.HAToken),
	}

	// Initialize MQTT client
	mqttClient, err := initMQTTClient(config)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize MQTT client: %w", err)
	}
	coordinator.mqttClient = mqttClient

	// Subscribe to play requests
	if err := coordinator.subscribeToPlayRequests(); err != nil {
		return nil, fmt.Errorf("failed to subscribe to MQTT topics: %w", err)
	}

	return coordinator, nil
}

func initMQTTClient(config *Config) (mqtt.Client, error) {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(config.MQTTBroker)
	opts.SetClientID(config.MQTTClientID)
	if config.MQTTUser != "" {
		opts.SetUsername(config.MQTTUser)
		opts.SetPassword(config.MQTTPass)
	}
	opts.SetAutoReconnect(true)
	opts.SetConnectRetry(true)
	opts.SetConnectRetryInterval(5 * time.Second)
	opts.SetKeepAlive(60 * time.Second)
	opts.SetPingTimeout(10 * time.Second)
	opts.SetConnectionLostHandler(func(client mqtt.Client, err error) {
		log.Printf("[MQTT] Connection lost: %v", err)
	})
	opts.SetOnConnectHandler(func(client mqtt.Client) {
		log.Printf("[MQTT] Connected to broker")
	})

	client := mqtt.NewClient(opts)
	token := client.Connect()
	if token.Wait() && token.Error() != nil {
		return nil, fmt.Errorf("failed to connect to MQTT broker: %w", token.Error())
	}
	return client, nil
}

func (c *Coordinator) subscribeToPlayRequests() error {
	token := c.mqttClient.Subscribe(mqttPlayTopic, 0, func(client mqtt.Client, msg mqtt.Message) {
		var req IntentRequest
		if err := json.Unmarshal(msg.Payload(), &req); err != nil {
			log.Printf("[MQTT] Failed to parse play request: %v", err)
			return
		}
		if err := c.processPlayRequest(req); err != nil {
			log.Printf("[MQTT] Failed to process play request: %v", err)
		}
	})

	if token.Wait() && token.Error() != nil {
		return fmt.Errorf("failed to subscribe to %s: %w", mqttPlayTopic, token.Error())
	}
	return nil
}

func (c *Coordinator) processPlayRequest(req IntentRequest) error {
	if req.Intent == "" || req.Location == "" {
		return fmt.Errorf("intent and location are required")
	}
	playlist, err := c.db.GetIntentPlaylist(req.Intent)
	if err != nil {
		return fmt.Errorf("intent not found: %w", err)
	}
	speakerEntity, err := c.db.GetLocationSpeaker(req.Location)
	if err != nil {
		return fmt.Errorf("location not found: %w", err)
	}
	return c.playMusicViaMQTT(speakerEntity, playlist)
}

func (c *Coordinator) HandlePlayIntent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req IntentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		c.sendError(w, http.StatusBadRequest, fmt.Sprintf("Invalid request body: %v", err))
		return
	}

	if req.Intent == "" || req.Location == "" {
		c.sendError(w, http.StatusBadRequest, "intent and location are required")
		return
	}

	playlist, err := c.db.GetIntentPlaylist(req.Intent)
	if err != nil {
		c.sendError(w, http.StatusNotFound, err.Error())
		return
	}

	speakerEntity, err := c.db.GetLocationSpeaker(req.Location)
	if err != nil {
		c.sendError(w, http.StatusNotFound, err.Error())
		return
	}

	if err := c.playMusicViaMQTT(speakerEntity, playlist); err != nil {
		c.sendError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to play music: %v", err))
		return
	}

	c.sendSuccess(w, fmt.Sprintf("Playing intent '%s' on '%s'", req.Intent, req.Location))
}

func (c *Coordinator) HandleIntents(w http.ResponseWriter, r *http.Request) {
	setCORSHeaders(w)

	switch r.Method {
	case http.MethodGet:
		intents, err := c.db.GetAllIntents()
		if err != nil {
			c.sendError(w, http.StatusInternalServerError, err.Error())
			return
		}
		json.NewEncoder(w).Encode(intents)

	case http.MethodPost:
		var intent Intent
		if err := json.NewDecoder(r.Body).Decode(&intent); err != nil {
			c.sendError(w, http.StatusBadRequest, fmt.Sprintf("Invalid request body: %v", err))
			return
		}

		playlists := intent.Playlists
		if len(playlists) == 0 && intent.Playlist != "" {
			playlists = []string{intent.Playlist}
		}

		if intent.Name == "" || len(playlists) == 0 {
			c.sendError(w, http.StatusBadRequest, "name and at least one playlist are required")
			return
		}

		if err := c.db.CreateIntent(intent.Name, playlists, ""); err != nil {
			c.sendError(w, http.StatusBadRequest, err.Error())
			return
		}
		c.sendSuccess(w, fmt.Sprintf("Intent '%s' created with %d playlist(s)", intent.Name, len(playlists)))

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (c *Coordinator) HandleIntent(w http.ResponseWriter, r *http.Request) {
	setCORSHeaders(w)

	name := r.URL.Path[len("/api/intents/"):]
	if name == "" {
		http.Error(w, "Intent name required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		intent, err := c.db.GetIntent(name)
		if err != nil {
			c.sendError(w, http.StatusNotFound, err.Error())
			return
		}
		json.NewEncoder(w).Encode(intent)

	case http.MethodPut:
		var intent Intent
		if err := json.NewDecoder(r.Body).Decode(&intent); err != nil {
			c.sendError(w, http.StatusBadRequest, fmt.Sprintf("Invalid request body: %v", err))
			return
		}

		playlistGroup := intent.PlaylistGroup
		playlists := intent.Playlists
		if playlistGroup == "" && len(playlists) == 0 && intent.Playlist != "" {
			playlists = []string{intent.Playlist}
		}

		if playlistGroup == "" && len(playlists) == 0 {
			c.sendError(w, http.StatusBadRequest, "either playlists or playlist_group is required")
			return
		}

		if err := c.db.UpdateIntent(name, playlists, playlistGroup); err != nil {
			c.sendError(w, http.StatusNotFound, err.Error())
			return
		}

		if playlistGroup != "" {
			c.sendSuccess(w, fmt.Sprintf("Intent '%s' updated with playlist group '%s'", name, playlistGroup))
		} else {
			c.sendSuccess(w, fmt.Sprintf("Intent '%s' updated with %d playlist(s)", name, len(playlists)))
		}

	case http.MethodDelete:
		if err := c.db.DeleteIntent(name); err != nil {
			c.sendError(w, http.StatusNotFound, err.Error())
			return
		}
		c.sendSuccess(w, fmt.Sprintf("Intent '%s' deleted", name))

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (c *Coordinator) HandleLocations(w http.ResponseWriter, r *http.Request) {
	setCORSHeaders(w, "GET", "POST", "OPTIONS")

	if r.Method == http.MethodOptions {
		handleOptions(w)
		return
	}

	switch r.Method {
	case http.MethodGet:
		locations, err := c.db.GetAllLocations()
		if err != nil {
			c.sendError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if locations == nil {
			locations = []Location{}
		}
		json.NewEncoder(w).Encode(locations)

	case http.MethodPost:
		var location Location
		if err := json.NewDecoder(r.Body).Decode(&location); err != nil {
			c.sendError(w, http.StatusBadRequest, fmt.Sprintf("Invalid request body: %v", err))
			return
		}
		if location.Name == "" || location.SpeakerEntity == "" {
			c.sendError(w, http.StatusBadRequest, "name and speaker_entity are required")
			return
		}
		if err := c.db.CreateLocation(location.Name, location.SpeakerEntity); err != nil {
			c.sendError(w, http.StatusBadRequest, err.Error())
			return
		}
		c.sendSuccess(w, fmt.Sprintf("Location '%s' created", location.Name))

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (c *Coordinator) HandleLocation(w http.ResponseWriter, r *http.Request) {
	setCORSHeaders(w)

	name := r.URL.Path[len("/api/locations/"):]
	if name == "" {
		http.Error(w, "Location name required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		location, err := c.db.GetLocation(name)
		if err != nil {
			c.sendError(w, http.StatusNotFound, err.Error())
			return
		}
		json.NewEncoder(w).Encode(location)

	case http.MethodPut:
		var location Location
		if err := json.NewDecoder(r.Body).Decode(&location); err != nil {
			c.sendError(w, http.StatusBadRequest, fmt.Sprintf("Invalid request body: %v", err))
			return
		}
		if location.SpeakerEntity == "" {
			c.sendError(w, http.StatusBadRequest, "speaker_entity is required")
			return
		}
		if err := c.db.UpdateLocation(name, location.SpeakerEntity); err != nil {
			c.sendError(w, http.StatusNotFound, err.Error())
			return
		}
		c.sendSuccess(w, fmt.Sprintf("Location '%s' updated", name))

	case http.MethodDelete:
		if err := c.db.DeleteLocation(name); err != nil {
			c.sendError(w, http.StatusNotFound, err.Error())
			return
		}
		c.sendSuccess(w, fmt.Sprintf("Location '%s' deleted", name))

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (c *Coordinator) HandleMediaPlayers(w http.ResponseWriter, r *http.Request) {
	setCORSHeaders(w, "GET", "OPTIONS")

	if r.Method == http.MethodOptions {
		handleOptions(w)
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	mediaPlayers, err := c.haClient.GetMediaPlayers()
	if err != nil {
		c.sendError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to fetch media players: %v", err))
		return
	}

	if mediaPlayers == nil {
		mediaPlayers = []MediaPlayer{}
	}
	json.NewEncoder(w).Encode(mediaPlayers)
}

func (c *Coordinator) HandleSyncLocations(w http.ResponseWriter, r *http.Request) {
	setCORSHeaders(w, "POST", "OPTIONS")

	if r.Method == http.MethodOptions {
		handleOptions(w)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	mediaPlayers, err := c.haClient.GetMediaPlayers()
	if err != nil {
		c.sendError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to fetch media players: %v", err))
		return
	}

	if len(mediaPlayers) == 0 {
		c.sendSuccess(w, "No media players found in Home Assistant")
		return
	}

	existingLocations, err := c.db.GetAllLocations()
	if err != nil {
		c.sendError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to fetch existing locations: %v", err))
		return
	}

	existingMap := make(map[string]bool, len(existingLocations))
	for _, loc := range existingLocations {
		existingMap[loc.Name] = true
	}

	created, skipped := 0, 0
	for _, mp := range mediaPlayers {
		locationName := strings.TrimPrefix(mp.EntityID, mediaPlayerPrefix)
		if existingMap[locationName] {
			skipped++
			continue
		}
		if err := c.db.CreateLocation(locationName, mp.EntityID); err != nil {
			continue
		}
		created++
	}

	c.sendSuccess(w, fmt.Sprintf("Synced locations: %d created, %d skipped", created, skipped))
}

func (c *Coordinator) HandlePlaylistGroups(w http.ResponseWriter, r *http.Request) {
	setCORSHeaders(w, "GET", "POST", "OPTIONS")

	if r.Method == http.MethodOptions {
		handleOptions(w)
		return
	}

	switch r.Method {
	case http.MethodGet:
		groups, err := c.db.GetAllPlaylistGroups()
		if err != nil {
			c.sendError(w, http.StatusInternalServerError, err.Error())
			return
		}
		json.NewEncoder(w).Encode(groups)

	case http.MethodPost:
		var group PlaylistGroup
		if err := json.NewDecoder(r.Body).Decode(&group); err != nil {
			c.sendError(w, http.StatusBadRequest, fmt.Sprintf("Invalid request body: %v", err))
			return
		}
		if group.Name == "" {
			c.sendError(w, http.StatusBadRequest, "name is required")
			return
		}
		if len(group.Playlists) == 0 {
			c.sendError(w, http.StatusBadRequest, "at least one playlist is required")
			return
		}
		if err := c.db.CreatePlaylistGroup(group.Name, group.Playlists); err != nil {
			c.sendError(w, http.StatusBadRequest, err.Error())
			return
		}
		c.sendSuccess(w, fmt.Sprintf("Playlist group '%s' created with %d playlist(s)", group.Name, len(group.Playlists)))

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (c *Coordinator) HandlePlaylistGroup(w http.ResponseWriter, r *http.Request) {
	setCORSHeaders(w, "GET", "PUT", "DELETE", "OPTIONS")

	if r.Method == http.MethodOptions {
		handleOptions(w)
		return
	}

	name := r.URL.Path[len("/api/playlist-groups/"):]
	if name == "" {
		http.Error(w, "Playlist group name required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		playlists, err := c.db.GetGroupPlaylists(name)
		if err != nil {
			c.sendError(w, http.StatusNotFound, err.Error())
			return
		}
		json.NewEncoder(w).Encode(PlaylistGroup{Name: name, Playlists: playlists})

	case http.MethodPut:
		var group PlaylistGroup
		if err := json.NewDecoder(r.Body).Decode(&group); err != nil {
			c.sendError(w, http.StatusBadRequest, fmt.Sprintf("Invalid request body: %v", err))
			return
		}
		if len(group.Playlists) == 0 {
			c.sendError(w, http.StatusBadRequest, "at least one playlist is required")
			return
		}
		if err := c.db.UpdatePlaylistGroup(name, group.Playlists); err != nil {
			c.sendError(w, http.StatusNotFound, err.Error())
			return
		}
		c.sendSuccess(w, fmt.Sprintf("Playlist group '%s' updated with %d playlist(s)", name, len(group.Playlists)))

	case http.MethodDelete:
		if err := c.db.DeletePlaylistGroup(name); err != nil {
			c.sendError(w, http.StatusNotFound, err.Error())
			return
		}
		c.sendSuccess(w, fmt.Sprintf("Playlist group '%s' deleted", name))

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (c *Coordinator) HandleAvailablePlaylists(w http.ResponseWriter, r *http.Request) {
	setCORSHeaders(w)

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	playlists, err := c.db.GetAllAvailablePlaylists()
	if err != nil {
		c.sendError(w, http.StatusInternalServerError, err.Error())
		return
	}
	json.NewEncoder(w).Encode(playlists)
}

func (c *Coordinator) sendSuccess(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(IntentResponse{
		Success: true,
		Message: message,
	})
}

func (c *Coordinator) sendError(w http.ResponseWriter, statusCode int, errorMsg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(IntentResponse{
		Success: false,
		Error:   errorMsg,
	})
}

type HAClient struct {
	baseURL string
	token   string
	client  *http.Client
}

func NewHAClient(baseURL, token string) *HAClient {
	return &HAClient{
		baseURL: baseURL,
		token:   token,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *Coordinator) playMusicViaMQTT(speakerEntity, playlist string) error {
	payload := map[string]interface{}{
		"entity_id":  speakerEntity,
		"media_id":   playlist,
		"media_type": "playlist",
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	token := c.mqttClient.Publish(mqttHATopic, 0, false, jsonData)
	if token.Wait() && token.Error() != nil {
		return fmt.Errorf("failed to publish MQTT message: %w", token.Error())
	}
	return nil
}

// MediaPlayer represents a Home Assistant media player entity
type MediaPlayer struct {
	EntityID   string `json:"entity_id"`
	Name       string `json:"name"`
	State      string `json:"state"`
	DeviceName string `json:"device_name,omitempty"`
}

func (c *HAClient) GetMediaPlayers() ([]MediaPlayer, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/api/states", c.baseURL), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.token))

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HA API returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var states []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&states); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	var mediaPlayers []MediaPlayer
	for _, state := range states {
		entityID, ok := state["entity_id"].(string)
		if !ok || !strings.HasPrefix(entityID, mediaPlayerPrefix) {
			continue
		}

		mp := MediaPlayer{EntityID: entityID}
		if attrs, ok := state["attributes"].(map[string]interface{}); ok {
			if name, ok := attrs["friendly_name"].(string); ok {
				mp.Name = name
			} else {
				mp.Name = strings.TrimPrefix(entityID, mediaPlayerPrefix)
			}
			if deviceName, ok := attrs["device_name"].(string); ok {
				mp.DeviceName = deviceName
			}
		}
		if stateVal, ok := state["state"].(string); ok {
			mp.State = stateVal
		}
		mediaPlayers = append(mediaPlayers, mp)
	}
	return mediaPlayers, nil
}

func main() {
	config := &Config{
		Port:         getEnv("PORT", defaultPort),
		DBPath:       getEnv("DB_PATH", defaultDBPath),
		HAURL:        getEnv("HA_URL", defaultHAURL),
		HAToken:      getEnv("HA_API_TOKEN", defaultHAToken),
		MAAPIURL:     getEnv("MA_API_URL", defaultMAAPIURL),
		MQTTBroker:   getEnv("MQTT_BROKER", defaultMQTTBroker),
		MQTTUser:     getEnv("MQTT_USER", defaultMQTTUser),
		MQTTPass:     getEnv("MQTT_PASS", defaultMQTTPass),
		MQTTClientID: getEnv("MQTT_CLIENT_ID", defaultMQTTClientID),
	}

	db, err := NewDatabase(config.DBPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	coordinator, err := NewCoordinator(db, config)
	if err != nil {
		log.Fatalf("Failed to initialize coordinator: %v", err)
	}
	defer coordinator.mqttClient.Disconnect(250)

	http.HandleFunc("/api/play", coordinator.HandlePlayIntent)
	http.HandleFunc("/play", coordinator.HandlePlayIntent)
	http.HandleFunc("/api/intents", coordinator.HandleIntents)
	http.HandleFunc("/api/intents/", coordinator.HandleIntent)
	http.HandleFunc("/api/locations", coordinator.HandleLocations)
	http.HandleFunc("/api/locations/", coordinator.HandleLocation)
	http.HandleFunc("/api/playlist-groups", coordinator.HandlePlaylistGroups)
	http.HandleFunc("/api/playlist-groups/", coordinator.HandlePlaylistGroup)
	http.HandleFunc("/api/available-playlists", coordinator.HandleAvailablePlaylists)
	http.HandleFunc("/api/media-players", coordinator.HandleMediaPlayers)
	http.HandleFunc("/api/sync-locations", coordinator.HandleSyncLocations)
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	fs := http.FileServer(http.Dir("./ui"))
	http.Handle("/", http.StripPrefix("/", fs))

	log.Printf("Server starting on port %s", config.Port)
	if err := http.ListenAndServe(":"+config.Port, nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
