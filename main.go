package main

import (
	"log"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"

	"openstack-reporter/internal/handlers"
	"openstack-reporter/internal/version"
)

func main() {
	// Print version information
	log.Println(version.GetFullVersionString())

	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using system environment variables")
	}

	// Initialize web server
	r := gin.Default()

	// Setup routes
	setupRoutes(r)

	// Start server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Starting server on port %s", port)
	log.Fatal(r.Run(":" + port))
}

func setupRoutes(r *gin.Engine) {
	// Initialize handlers
	handler := handlers.NewHandler()

	// Static files
	r.Static("/static", "./web/static")
	r.LoadHTMLGlob("web/templates/*")

	// API routes
	api := r.Group("/api")
	{
		api.GET("/resources", handler.GetResources)
		api.POST("/refresh", handler.RefreshResources)
		api.GET("/export/pdf", handler.ExportToPDF)
		api.GET("/status", handler.GetReportStatus)
		api.GET("/version", getVersion)
	}

	// Web routes
	r.GET("/", indexHandler)
}

func indexHandler(c *gin.Context) {
	c.HTML(200, "index.html", gin.H{
		"title":   "OpenStack Resources Report",
		"version": version.GetVersionString(),
	})
}

func getVersion(c *gin.Context) {
	c.JSON(200, version.Get())
}
