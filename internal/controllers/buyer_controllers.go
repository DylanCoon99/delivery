package controllers



import (
	//"os"
	//"log"
	//"time"
	"net/http"
	"strconv"
	"encoding/json"
	"database/sql"
	"github.com/google/uuid"
	//"golang.org/x/crypto/bcrypt"
	//"github.com/joho/godotenv"
	"github.com/gin-gonic/gin"
	//"github.com/DylanCoon99/lead_delivery_app/backend/internal/auth"
	"github.com/DylanCoon99/lead_delivery_app/backend/internal/utils"
	"github.com/DylanCoon99/lead_delivery_app/backend/internal/database/queries"
)




func (cfg *ApiConfig) ListAllBuyers(c *gin.Context) {

	header := c.GetHeader("Authorization")

	tenantId, err := utils.ExtractTokenTenantID(header)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get tenant id"})
		return
	}


	// Optional query params for pagination
	limit := int32(50) // default
	offset := int32(0) // default
	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil {
			limit = int32(parsed)
		}
	}
	if o := c.Query("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil {
			offset = int32(parsed)
		}
	}

	buyers, err := cfg.DBQueries.ListBuyers(c, queries.ListBuyersParams{
		TenantID: tenantId,
		Limit:    limit,
		Offset:   offset,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list buyers"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"buyers": buyers})
}




func (cfg *ApiConfig) GetBuyerByID(c *gin.Context) {

	header := c.GetHeader("Authorization")

	tenantId, err := utils.ExtractTokenTenantID(header)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get tenant id"})
		return
	}


	idParam := c.Param("id")
	buyerId, err := uuid.Parse(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid buyer id"})
		return
	}

	buyer, err := cfg.DBQueries.GetBuyerByID(c, queries.GetBuyerByIDParams{
		ID:       buyerId,
		TenantID: tenantId,
	})
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "buyer not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"buyer": buyer})
}





func (cfg *ApiConfig) GetBuyerForCampaign(c *gin.Context) {

	header := c.GetHeader("Authorization")

	tenantId, err := utils.ExtractTokenTenantID(header)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get tenant id"})
		return
	}


	idParam := c.Param("campaign_id")
	campaignId, err := uuid.Parse(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid campaign id"})
		return
	}


	campaign, err := cfg.DBQueries.GetCampaignByID(c, queries.GetCampaignByIDParams{
		ID:       campaignId,
		TenantID: tenantId,
	})
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "campaign not found"})
		return
	}

	buyerId := campaign.BuyerID.UUID


	buyer, err := cfg.DBQueries.GetBuyerByID(c, queries.GetBuyerByIDParams{
		ID:       buyerId,
		TenantID: tenantId,
	})
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "buyer not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"buyer": buyer})
}



func (cfg *ApiConfig) CreateBuyer(c *gin.Context) {
    header := c.GetHeader("Authorization")

    tenantId, err := utils.ExtractTokenTenantID(header)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get tenant id"})
        return
    }

    var req struct {
        Name            string `json:"name" binding:"required"`
        ContactEmail    string `json:"contact_email"`
        DeliveryMethod  string `json:"delivery_method" binding:"required"`
    }

    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    email := sql.NullString{
        String: req.ContactEmail,
        Valid:  req.ContactEmail != "",
    }

    // 1️⃣ Create Buyer (NO preferred_delivery_method here)
    buyer, err := cfg.DBQueries.CreateBuyer(c, queries.CreateBuyerParams{
        TenantID:     tenantId,
        Name:         req.Name,
        ContactEmail: email,
    })
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{
            "error":   "failed to create buyer",
            "message": err.Error(),
        })
        return
    }

    // 2️⃣ Create Delivery Method
    deliveryMethodID := uuid.New()

    _, err = cfg.DBQueries.CreateDeliveryMethod(c, queries.CreateDeliveryMethodParams{
        ID:         deliveryMethodID,
        TenantID:   tenantId,
        BuyerID:    uuid.NullUUID{UUID: buyer.ID, Valid: true},
        MethodType: sql.NullString{
            String: req.DeliveryMethod,
            Valid:  req.DeliveryMethod != "",
        },
        Config:  json.RawMessage(`{}`), // empty config for now
        Column6: nil,
    })
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{
            "error":   "failed to create delivery method",
            "message": err.Error(),
        })
        return
    }

    c.JSON(http.StatusCreated, gin.H{
        "buyer":             buyer,
        "delivery_method_id": deliveryMethodID,
    })
}




func (cfg *ApiConfig) UpdateBuyer(c *gin.Context) {
    header := c.GetHeader("Authorization")

    tenantId, err := utils.ExtractTokenTenantID(header)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get tenant id"})
        return
    }

    // --- Buyer ID ---
    idParam := c.Param("id")
    buyerID, err := uuid.Parse(idParam)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid buyer id"})
        return
    }

    // --- Request Body ---
    var req struct {
        Name           string `json:"name"`
        ContactEmail   string `json:"contact_email"`
        DeliveryMethod string `json:"delivery_method"`
        IsActive       bool   `json:"is_active"`
    }

    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    email := sql.NullString{
        String: req.ContactEmail,
        Valid:  req.ContactEmail != "",
    }

    // --- 1️⃣ Update Buyer in DB (no preferred_delivery_method) ---
    buyer, err := cfg.DBQueries.UpdateBuyer(c, queries.UpdateBuyerParams{
        ID:           buyerID,
        TenantID:     tenantId,
        Name:         req.Name,
        ContactEmail: email,
        IsActive:     sql.NullBool{
        	Bool:  req.IsActive,
        	Valid: true,
        },
    })
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{
            "error":   err.Error(),
            "message": "failed to update buyer",
        })
        return
    }

    /*
    type DeliveryMethod struct {
		ID         uuid.UUID
		TenantID   uuid.UUID
		BuyerID    uuid.NullUUID
		MethodType sql.NullString
		Config     json.RawMessage
		IsActive   sql.NullBool
		CreatedAt  sql.NullTime
		UpdatedAt  sql.NullTime
	}

    2025/12/02 01:34:38 Here is the delivery method: {0b6d0f57-e490-4a15-8b0b-a1eb5ea1d0e7 caf1a1bc-d0a1-4503-8c38-006e033fccf2 {51235dd8-95a0-47cb-9279-6f298696c97e true} {api true} [123 125] {true true} {2025-12-02 01:18:45.924527 +0000 +0000 true} {2025-12-02 01:18:45.924527 +0000 +0000 true}}
    */



    // --- 2️⃣ Get existing delivery method for buyer ---
    deliveryMethod, err := cfg.DBQueries.GetDeliveryMethodByBuyerID(c, queries.GetDeliveryMethodByBuyerIDParams{
    	BuyerID: uuid.NullUUID{
	        UUID:  buyerID,
	        Valid: true,
	    },
    	TenantID: tenantId,
    })

    //log.Printf("Here is the delivery method: %v", deliveryMethod)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{
            "error":   err.Error(),
            "message": "failed to fetch delivery method",
        })
        return
    }

    // --- 3️⃣ Update Delivery Method ---
    _, err = cfg.DBQueries.UpdateDeliveryMethod(c, queries.UpdateDeliveryMethodParams{
        ID: deliveryMethod.ID,
        MethodType: sql.NullString{
            String: req.DeliveryMethod,
            Valid:  req.DeliveryMethod != "",
        },
        Config: deliveryMethod.Config, // do NOT modify
        IsActive: sql.NullBool{
        	Bool:  req.IsActive,
        	Valid: true,
        },
		TenantID: tenantId,
    })
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{
            "error":   err.Error(),
            "message": "failed to update delivery method",
        })
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "message": "successfully updated the buyer",
        "buyer":   buyer,
    })
}


func (cfg *ApiConfig) DeleteBuyer(c *gin.Context) {

	header := c.GetHeader("Authorization")

	tenantId, err := utils.ExtractTokenTenantID(header)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get tenant id"})
		return
	}


	idParam := c.Param("id")
	buyerId, err := uuid.Parse(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid buyer id"})
		return
	}

	err = cfg.DBQueries.DeleteBuyer(c, queries.DeleteBuyerParams{
		ID:       buyerId,
		TenantID: tenantId,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete buyer"})
		return
	}


	c.JSON(http.StatusOK, gin.H{"message": "buyer deleted"})
}
