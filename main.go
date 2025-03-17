package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Transaction struct {
	ID          int       `json:"id"`
	Date        time.Time `json:"date"`
	Description string    `json:"description"`
	Amount      float64   `json:"amount"`
	Type        string    `json:"type"`
	CreatedAt   time.Time `json:"created_at"`
}

type Job struct {
	JobID     string    `json:"job_id"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type API struct {
	db     *pgxpool.Pool
	router *gin.Engine
}

func NewAPI(db *pgxpool.Pool) *API {
	api := &API{
		db:     db,
		router: gin.Default(),
	}
	api.setupRoutes()
	return api
}

func (api *API) setupRoutes() {
	// Enable CORS
	api.router.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	// Transaction endpoints
	api.router.GET("/transactions", api.getTransactions)
	api.router.GET("/transactions/:id", api.getTransaction)
	api.router.GET("/stats", api.getStats)
	api.router.DELETE(("/transactions/:id"), api.deleteTransaction)
	api.router.DELETE("/jobs/most-recent", api.deleteMostRecentJob)
}

func (api *API) getTransactions(c *gin.Context) {
	rows, err := api.db.Query(context.Background(),
		"SELECT id, date, description, amount, type, created_at FROM transactions ORDER BY date DESC")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var transactions []Transaction
	for rows.Next() {
		var t Transaction
		if err := rows.Scan(&t.ID, &t.Date, &t.Description, &t.Amount, &t.Type, &t.CreatedAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		transactions = append(transactions, t)
	}

	c.JSON(http.StatusOK, transactions)
}

func (api *API) getTransaction(c *gin.Context) {
	id := c.Param("id")
	var t Transaction

	err := api.db.QueryRow(context.Background(),
		"SELECT id, date, description, amount, type, created_at FROM transactions WHERE id = $1", id).
		Scan(&t.ID, &t.Date, &t.Description, &t.Amount, &t.Type, &t.CreatedAt)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Transaction not found"})
		return
	}

	c.JSON(http.StatusOK, t)
}

func (api *API) getStats(c *gin.Context) {
	stats := struct {
		TotalTransactions int     `json:"total_transactions"`
		TotalDebits       float64 `json:"total_debits"`
		TotalCredits      float64 `json:"total_credits"`
	}{}

	// Get transaction counts and totals
	err := api.db.QueryRow(context.Background(), "SELECT COUNT(*) FROM transactions").Scan(&stats.TotalTransactions)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	err = api.db.QueryRow(context.Background(),
		"SELECT COALESCE(SUM(amount), 0) FROM transactions WHERE type = 'debit'").Scan(&stats.TotalDebits)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	err = api.db.QueryRow(context.Background(),
		"SELECT COALESCE(SUM(amount), 0) FROM transactions WHERE type = 'credit'").Scan(&stats.TotalCredits)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, stats)
}

func (api *API) deleteTransaction(c *gin.Context) {
	// Delete a transaction
	id := c.Param("id")

	result, err := api.db.Exec(context.Background(), "DELETE FROM transactions WHERE id = $1", id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if result.RowsAffected() == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Transaction not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Transaction deleted"})
}

func (api *API) deleteMostRecentJob(c *gin.Context) {
	// Delete transaction done by most recent job by getting job_id of most recent transacion and deleting all transactions with same job_id
	var jobID string
	err := api.db.QueryRow(context.Background(), "SELECT job_id FROM transactions ORDER BY created_at DESC LIMIT 1").Scan(&jobID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	result, err := api.db.Exec(context.Background(), "DELETE FROM transactions WHERE job_id = $1", jobID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if result.RowsAffected() == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Transaction not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Most recent job transactions deleted"})
}

func (api *API) Run(addr string) error {
	return api.router.Run(addr)
}

// Main function would look like this
func main() {
	dbURL := "postgresql://junpark@localhost:5432/bankstatements"
	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		log.Fatalf("Unable to connect to database: %v\n", err)
	}
	defer pool.Close()

	api := NewAPI(pool)
	api.Run(":8050")
}
