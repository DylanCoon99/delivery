package controllers


import (
	"log"
	"net/http"
	//"encoding/json"
	//"database/sql"
	//"github.com/google/uuid"
	"github.com/gin-gonic/gin"
	"github.com/DylanCoon99/lead_delivery_app/backend/internal/database/queries"
	"github.com/DylanCoon99/lead_delivery_app/backend/internal/email"
)


type ApiConfig struct {
	DBQueries   *queries.Queries
	EmailClient *email.EmailService
}


func Test(c *gin.Context) {

	log.Println("Test endpoint")

	c.JSON(http.StatusOK, gin.H{
      "message": "pong",
    })
    return

} 


