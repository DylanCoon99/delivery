package middleware


import (
	//"os"
	"log"
	"net/http"
	//"strings"
	"github.com/gin-gonic/gin"
	"github.com/DylanCoon99/lead_delivery_app/backend/internal/utils"
	"github.com/DylanCoon99/lead_delivery_app/backend/internal/auth"
)



func JwtSuperAdminMiddleware() gin.HandlerFunc {


	return func(c *gin.Context) {

		log.Println("SuperAdmin middleware triggered")

		header := c.GetHeader("Authorization")


		_, err := auth.ValidateJWT(header)

		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			c.Abort()
		}


		role, err := utils.ExtractTokenRole(header)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			log.Printf("SuperAdmin middleware failed to get token role. Role extracted: %v", role)
			c.Abort()
			return
		}

		// Check role
		if role != "super_admin" {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			log.Println("SuperAdmin middleware failed validate role.")

			c.Abort()
			return
		}


		// Store claims if needed downstream
		//c.Set("claims", claims)
		c.Next()
	}
}


func JwtAdminMiddleware() gin.HandlerFunc {


	return func(c *gin.Context) {

		log.Println("Admin middleware triggered")

		header := c.GetHeader("Authorization")


		_, err := auth.ValidateJWT(header)

		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			c.Abort()
		}


		role, err := utils.ExtractTokenRole(header)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			log.Printf("Admin middleware failed to get token role. Role extracted: %v", role)
			c.Abort()
			return
		}

		// Check role
		if role != "super_admin" && role != "admin" {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			log.Println("Admin middleware failed validate role.")

			c.Abort()
			return
		}


		// Store claims if needed downstream
		//c.Set("claims", claims)
		c.Next()
	}
}


func JwtSupplierMiddleware() gin.HandlerFunc {


	return func(c *gin.Context) {

		log.Println("Supplier middleware triggered")

		header := c.GetHeader("Authorization")


		_, err := auth.ValidateJWT(header)

		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			c.Abort()
		}


		role, err := utils.ExtractTokenRole(header)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			log.Printf("Supplier middleware failed to get token role. Role extracted: %v", role)
			c.Abort()
			return
		}

		// Check role
		if role != "super_admin" && role != "admin" && role != "supplier" {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			log.Println("Supplier middleware failed validate role.")

			c.Abort()
			return
		}


		// Store claims if needed downstream
		//c.Set("claims", claims)
		c.Next()
	}
}