package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/gorilla/handlers"

	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
)

const (
	host     = "localhost"
	port     = 5432
	user     = "postgres"
	password = "yourpassword"
	dbname   = "iot_devices"
)

var db *sql.DB

func main() {
	var err error
	psqlInfo := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)

	db, err = sql.Open("postgres", psqlInfo)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	err = db.Ping()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Successfully connected to database!")

	createTables()

	r := mux.NewRouter()
	r.HandleFunc("/api/device/status", createDeviceStatus).Methods("POST")
	r.HandleFunc("/api/device/status/{serialNo}", getLatestDeviceStatus).Methods("GET")

	corsOpts := handlers.CORS(
		handlers.AllowedOrigins([]string{"*"}), // In production, specify exact origins
		handlers.AllowedMethods([]string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}),
		handlers.AllowedHeaders([]string{"Content-Type", "Authorization"}),
		handlers.AllowCredentials(),
	)

	loggedRouter := handlers.LoggingHandler(os.Stdout, r)

	// Apply CORS middleware and logging to router
	http.Handle("/", corsOpts(loggedRouter))

	fmt.Println("Server started at http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func createTables() {
	query := `
    CREATE TABLE IF NOT EXISTS device_status (
        id SERIAL PRIMARY KEY,
        serial_no TEXT NOT NULL,
        name TEXT,
        timestamp TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
        data JSONB NOT NULL
    );

    CREATE INDEX IF NOT EXISTS idx_device_status_serial_no ON device_status(serial_no);
    CREATE INDEX IF NOT EXISTS idx_device_status_timestamp ON device_status(timestamp);
    `

	_, err := db.Exec(query)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Tables created or already exist")
}

func createDeviceStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var status map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&status); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	serialNo, ok := status["serialNo"].(string)
	if !ok {
		http.Error(w, "serialNo is required", http.StatusBadRequest)
		return
	}

	// Extract device name from the request
	name, _ := status["name"].(string)

	query := `INSERT INTO device_status (serial_no, name, data) VALUES ($1, $2, $3) RETURNING id`
	var id int
	dataJSON, err := json.Marshal(status)
	if err != nil {
		http.Error(w, "Failed to process data", http.StatusInternalServerError)
		return
	}

	err = db.QueryRow(query, serialNo, name, dataJSON).Scan(&id)
	if err != nil {
		http.Error(w, "Failed to insert record", http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{"id": id})
}

func getLatestDeviceStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	vars := mux.Vars(r)
	serialNo := vars["serialNo"]

	query := `SELECT id, serial_no, name, timestamp, data FROM device_status WHERE serial_no = $1 ORDER BY timestamp DESC LIMIT 1`
	var id int
	var serial string
	var name sql.NullString
	var timestamp string
	var dataJSON []byte

	err := db.QueryRow(query, serialNo).Scan(&id, &serial, &name, &timestamp, &dataJSON)
	if err != nil {
		http.Error(w, "Device status not found", http.StatusNotFound)
		return
	}

	var data map[string]interface{}
	if err := json.Unmarshal(dataJSON, &data); err != nil {
		http.Error(w, "Failed to parse data", http.StatusInternalServerError)
		return
	}

	// Use the device name if available
	deviceName := ""
	if name.Valid {
		deviceName = name.String
	}

	response := map[string]interface{}{
		"id":        id,
		"serialNo":  serial,
		"name":      deviceName,
		"timestamp": timestamp,
		"data":      data,
	}

	json.NewEncoder(w).Encode(response)
}
