package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	_ "github.com/lib/pq"
)

type Config struct {
	DBConnString string
	ServerPort   string
}

type App struct {
	DB     *sql.DB
	Logger *log.Logger
	Config Config
}

type Subscription struct {
	ID          int    `json:"id"`
	ServiceName string `json:"service_name"`
	Price       int    `json:"price"`
	UserID      string `json:"user_id"`
	StartDate   string `json:"start_date"`
}

func main() {
	logger := log.New(os.Stdout, "[API] ", log.LstdFlags)

	cfg := Config{
		DBConnString: getEnv("DATABASE_URL", "host=localhost port=5432 user=postgres password=x dbname=test sslmode=disable"),
		ServerPort:   getEnv("SERVER_PORT", "8080"),
	}

	db, err := sql.Open("postgres", cfg.DBConnString)
	if err != nil {
		logger.Fatal(err)
	}
	defer db.Close()

	db.SetMaxOpenConns(10)
	db.SetConnMaxLifetime(time.Minute * 5)

	if err := db.Ping(); err != nil {
		logger.Fatal("Cannot connect to DB:", err)
	}
	logger.Println("Database connected")

	app := &App{
		DB:     db,
		Logger: logger,
		Config: cfg,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/subscriptions", app.list)
	mux.HandleFunc("GET /api/subscriptions/id", app.read)
	mux.HandleFunc("POST /api/subscriptions", app.create)
	mux.HandleFunc("PUT /api/subscriptions", app.update)
	mux.HandleFunc("DELETE /api/subscriptions/id", app.delete)
	mux.HandleFunc("POST /api/sumCost", app.sumCost)

	server := &http.Server{
		Addr:    ":" + app.Config.ServerPort,
		Handler: mux,
	}

	logger.Println("Server starting on port", app.Config.ServerPort)
	logger.Fatal(server.ListenAndServe())
}

func (app *App) list(w http.ResponseWriter, r *http.Request) {
	query := "SELECT service_name, price, user_id, start_date FROM subscriptions"
	rows, err := app.DB.Query(query)
	if err != nil {
		app.Logger.Println("Query error:", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var subscriptions []Subscription
	for rows.Next() {
		var s Subscription
		if err := rows.Scan(&s.ServiceName, &s.Price, &s.UserID, &s.StartDate); err != nil {
			app.Logger.Println("Scan error:", err)
			continue
		}
		subscriptions = append(subscriptions, s)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(subscriptions)
}

func (app *App) read(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	query := "SELECT service_name, price, user_id, start_date FROM subscriptions WHERE id = $1"
	rows, err := app.DB.Query(query, id)
	if err != nil {
		app.Logger.Println("Query error:", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var subscriptions []Subscription
	for rows.Next() {
		var s Subscription
		if err := rows.Scan(&s.Price, &s.UserID, &s.StartDate); err != nil {
			app.Logger.Println("Scan error:", err)
			continue
		}
		subscriptions = append(subscriptions, s)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(subscriptions)
}

func (app *App) create(w http.ResponseWriter, r *http.Request) {
	var s Subscription
	if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	query := "INSERT INTO subscriptions (service_name, price, user_id, start_date) VALUES($1, $2, $3, TO_DATE($4, 'MM-YYYY'))"
	_, err := app.DB.Exec(query, s.ServiceName, s.Price, s.UserID, s.StartDate)

	if err != nil {
		app.Logger.Println("Insert error:", err)
		http.Error(w, "Conflict or Internal Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(s)
}

func (app *App) update(w http.ResponseWriter, r *http.Request) {
	var s Subscription
	if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	query := "UPDATE subscriptions SET service_name = $2, price = $3, user_id = $4, start_date = TO_DATE($5, 'MM-YYYY') WHERE id = $1"
	result, err := app.DB.Exec(query, s.ID, s.ServiceName, s.Price, s.UserID, s.StartDate)
	if err != nil {
		app.Logger.Println("Update error:", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		http.Error(w, "subscriptions not found", http.StatusNotFound)
		return
	}

	fmt.Fprintf(w, "User updated successfully")

}

func (app *App) delete(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	query := "DELETE FROM subscriptions WHERE id = $1"
	res, err := app.DB.Exec(query, id)
	if err != nil {
		app.Logger.Println("Delete error:", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	count, _ := res.RowsAffected()
	fmt.Printf("Deleted %d rows\n", count)

}

func (app *App) sumCost(w http.ResponseWriter, r *http.Request) {
	var sum float64
	var s Subscription
	if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	query := "SELECT SUM(price) FROM subscriptions WHERE service_name=$1 AND user_id=$2 AND start_date=TO_DATE($3, 'MM-YYYY')"
	err := app.DB.QueryRow(query, s.ServiceName, s.UserID, s.StartDate).Scan(&sum)
	if err != nil {
		app.Logger.Println("Summ Cost:", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sum)
}

func getEnv(key, defaultValue string) string {
	fmt.Println(key, defaultValue)
	if value, exists := os.LookupEnv(key); exists {
		fmt.Println(value)
		return value
	}
	return defaultValue
}
