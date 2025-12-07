package controllers


import (
	//"os"
	//"log"
	"time"
	"errors"
	"net/http"
	//"strconv"
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



func (cfg *ApiConfig) GetDeliveryForCampaign(c *gin.Context) {

	header := c.GetHeader("Authorization")

	tenantId, err := utils.ExtractTokenTenantID(header)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get tenant id"})
		return
	}

	// Optional query params
	campaignIDStr := c.Param("campaign_id")

	campaignId, err := uuid.Parse(campaignIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid campaign_id"})
		return
	}

	// Run the query
	delivery, err := cfg.DBQueries.GetDeliveryForCampaign(c, queries.GetDeliveryForCampaignParams{
		TenantID:   tenantId,
		CampaignID: campaignId,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
	        // Return null instead of an error
	        c.JSON(http.StatusOK, gin.H{"delivery": nil})
	        return
	    }

		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get delivery", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"delivery": delivery})


}



func (cfg *ApiConfig) GetDeliveryByID(c *gin.Context) {



	header := c.GetHeader("Authorization")

	tenantId, err := utils.ExtractTokenTenantID(header)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get tenant id"})
		return
	}

	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid delivery ID"})
		return
	}

	delivery, err := cfg.DBQueries.GetDeliveryByID(c, queries.GetDeliveryByIDParams{
		ID:       id,
		TenantID: tenantId,
	})
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "delivery not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch delivery"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"delivery": delivery})

}



func (cfg *ApiConfig) ScheduleDelivery(c *gin.Context) {


	header := c.GetHeader("Authorization")

	tenantId, err := utils.ExtractTokenTenantID(header)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get tenant id"})
		return
	}

	campaignIdStr := c.Param("campaign_id")
	campaignId, err := uuid.Parse(campaignIdStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid delivery ID"})
		return
	}

	var req struct {
		LeadBatchID  uuid.UUID  `json:"lead_batch_id" binding:"required"`
		ScheduledAt  time.Time  `json:"scheduled_at"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	leadBatchId := req.LeadBatchID

	scheduledAt := sql.NullTime{
		Time:  req.ScheduledAt,
		Valid: true,
	}


	delivery, err := cfg.DBQueries.ScheduleDelivery(c, queries.ScheduleDeliveryParams{
		TenantID:    tenantId,
		CampaignID:  campaignId,
		LeadBatchID: leadBatchId,
		ScheduledAt: scheduledAt,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create delivery", "message": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"delivery": delivery})
}




func (cfg *ApiConfig) UpdateDeliveryStatus(c *gin.Context) {


	header := c.GetHeader("Authorization")

	tenantId, err := utils.ExtractTokenTenantID(header)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get tenant id"})
		return
	}

	// Delivery ID from URL
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid delivery ID"})
		return
	}

	// Parse request body
	var req struct {
		Status string `json:"status" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}


	// Ensure the delivery exists first (optional but recommended)
	_, err = cfg.DBQueries.GetDeliveryByID(c, queries.GetDeliveryByIDParams{
		ID:       id,
		TenantID: tenantId,
	})
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "delivery not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to verify delivery"})
		return
	}

	// Perform the update
	err = cfg.DBQueries.UpdateDeliveryStatus(c, queries.UpdateDeliveryStatusParams{
		ID:       id,
		TenantID: tenantId,
		Status:   utils.SqlNullString(req.Status),
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update delivery status"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "delivery status updated successfully",
		"status":  req.Status,
	})


}


func (cfg *ApiConfig) DeleteDelivery(c *gin.Context) {

	header := c.GetHeader("Authorization")

	tenantId, err := utils.ExtractTokenTenantID(header)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get tenant id"})
		return
	}


	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid delivery ID"})
		return
	}

	// Check that the delivery exists before deleting
	_, err = cfg.DBQueries.GetDeliveryByID(c, queries.GetDeliveryByIDParams{
		ID:       id,
		TenantID: tenantId,
	})
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "delivery not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to verify delivery"})
		return
	}

	err = cfg.DBQueries.DeleteDeliveryByID(c, queries.DeleteDeliveryByIDParams{
		ID:       id,
		TenantID: tenantId,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete delivery"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "delivery deleted successfully"})


}


func (cfg *ApiConfig) UpdateDeliveryMethod(c *gin.Context) {
    header := c.GetHeader("Authorization")

    // Parse delivery_method ID from path
    deliveryMethodIDStr := c.Param("id")
    deliveryMethodID, err := uuid.Parse(deliveryMethodIDStr)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid delivery_method id"})
        return
    }

	tenantId, err := utils.ExtractTokenTenantID(header)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get tenant id"})
		return
	}

    // Parse request body
    var req struct {
        MethodType *string         `json:"method_type"`
        Config     json.RawMessage `json:"config"`
        IsActive   *bool           `json:"is_active"`
    }

    if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
    }

    // Convert request fields to sql.Null types
    params := queries.UpdateDeliveryMethodParams{
        ID: deliveryMethodID,
        MethodType: sql.NullString{
            String: utils.SafeString(req.MethodType),
            Valid:  req.MethodType != nil,
        },
        Config: req.Config, // JSONB always valid, handled by COALESCE in SQL
        IsActive: sql.NullBool{
            Bool:  utils.SafeBool(req.IsActive),
            Valid: req.IsActive != nil,
        },
        TenantID: tenantId,
    }

    // Execute update
    updatedMethod, err := cfg.DBQueries.UpdateDeliveryMethod(c, params)
    if err != nil {
        if err == sql.ErrNoRows {
           	c.JSON(http.StatusNotFound, gin.H{"error": "delivery method not found"})
            return
        }
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to update delivery method"})
		return
    }

    // Success
    c.JSON(http.StatusOK, gin.H{"delivery_method": updatedMethod})

}

/*
const getDeliveryMethodByBuyerID = `-- name: GetDeliveryMethodByBuyerID :one
SELECT id, tenant_id, buyer_id, method_type, config, is_active, created_at, updated_at FROM delivery_methods
WHERE buyer_id = $1
  AND tenant_id = $2
LIMIT 1
`

type GetDeliveryMethodByBuyerIDParams struct {
	BuyerID  uuid.NullUUID
	TenantID uuid.UUID
}

func (q *Queries) GetDeliveryMethodByBuyerID(ctx context.Context, arg GetDeliveryMethodByBuyerIDParams) (DeliveryMethod, error) {
	row := q.db.QueryRowContext(ctx, getDeliveryMethodByBuyerID, arg.BuyerID, arg.TenantID)
	var i DeliveryMethod
	err := row.Scan(
		&i.ID,
		&i.TenantID,
		&i.BuyerID,
		&i.MethodType,
		&i.Config,
		&i.IsActive,
		&i.CreatedAt,
		&i.UpdatedAt,
	)
	return i, err
}

*/




// GetDeliveryMethodByBuyerID
func (cfg *ApiConfig) GetBuyerDeliveryMethod(c *gin.Context) {

	header := c.GetHeader("Authorization")

	tenantId, err := utils.ExtractTokenTenantID(header)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get tenant id"})
		return
	}

	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid delivery ID"})
		return
	}


	delivery, err := cfg.DBQueries.GetDeliveryMethodByBuyerID(c, queries.GetDeliveryMethodByBuyerIDParams{
		BuyerID:       utils.NullUUID(id),
		TenantID: tenantId,
	})
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "delivery method not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch delivery method"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"delivery_method": delivery})


}


func (cfg *ApiConfig) TriggerDelivery(c *gin.Context) {



}