package controllers


import (
	//"os"
	//"log"
	"time"
	"net/http"
	"strconv"
	//"encoding/json"
	"database/sql"
	"github.com/google/uuid"
	//"golang.org/x/crypto/bcrypt"
	//"github.com/joho/godotenv"
	"github.com/gin-gonic/gin"
	//"github.com/DylanCoon99/lead_delivery_app/backend/internal/auth"
	"github.com/DylanCoon99/lead_delivery_app/backend/internal/utils"
	"github.com/DylanCoon99/lead_delivery_app/backend/internal/database/queries"
)




func (cfg *ApiConfig) ListCampaigns(c *gin.Context) {

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

	campaigns, err := cfg.DBQueries.ListCampaigns(c, queries.ListCampaignsParams{
		TenantID: tenantId,
		Limit:    limit,
		Offset:   offset,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list campaigns", "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"campaigns": campaigns})
}




func (cfg *ApiConfig) GetCampaignByID(c *gin.Context) {

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

	c.JSON(http.StatusOK, gin.H{"campaign": campaign})
}



func (cfg *ApiConfig) CreateCampaign(c *gin.Context) {
	header := c.GetHeader("Authorization")

	tenantId, err := utils.ExtractTokenTenantID(header)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get tenant id"})
		return
	}

	var req struct {
		Name             string     `json:"name" binding:"required"`
		BuyerID          uuid.UUID  `json:"buyer_id"`
		DeliverySchedule string     `json:"delivery_schedule"`
		Description      string     `json:"description"`
		StartDate        *time.Time `json:"start_date"`
		EndDate          *time.Time `json:"end_date"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	buyerID := uuid.NullUUID{
		UUID:  req.BuyerID,
		Valid: req.BuyerID != uuid.Nil,
	}

	deliverySchedule := sql.NullString{
		String: req.DeliverySchedule,
		Valid:  req.DeliverySchedule != "",
	}

	description := sql.NullString{
		String: req.Description,
		Valid:  req.Description != "",
	}

	startDate := sql.NullTime{
		Time:  time.Time{},
		Valid: false,
	}
	if req.StartDate != nil {
		startDate.Time = *req.StartDate
		startDate.Valid = true
	}

	endDate := sql.NullTime{
		Time:  time.Time{},
		Valid: false,
	}
	if req.EndDate != nil {
		endDate.Time = *req.EndDate
		endDate.Valid = true
	}

	campaign, err := cfg.DBQueries.CreateCampaign(c, queries.CreateCampaignParams{
		TenantID:         tenantId,
		Name:             req.Name,
		BuyerID:          buyerID,
		DeliverySchedule: deliverySchedule,
		Description:      description,
		StartDate:        startDate,
		EndDate:          endDate,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create campaign", "message": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"campaign": campaign})
}




func (cfg *ApiConfig) UpdateCampaignStatus(c *gin.Context) {

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

	var req struct {
		IsActive   bool    `json:"is_active"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	active := sql.NullBool{Bool: req.IsActive, Valid: true}


	err = cfg.DBQueries.UpdateCampaignStatus(c, queries.UpdateCampaignStatusParams{
		ID:       campaignId,
		TenantID: tenantId,
		IsActive: active,
	})
	
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "failed to update campaign status"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "successfully updated the campaign status"})
}



func (cfg *ApiConfig) DeleteCampaign(c *gin.Context) {

	header := c.GetHeader("Authorization")

	tenantId, err := utils.ExtractTokenTenantID(header)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get tenant id"})
		return
	}

	idParam := c.Param("id")
	campaignID, err := uuid.Parse(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid tenant id"})
		return
	}

	err = cfg.DBQueries.DeleteCampaign(c, queries.DeleteCampaignParams{
		ID:       campaignID,
		TenantID: tenantId, 
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete campaign"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "campaign deleted"})

}



func (cfg *ApiConfig) GetSuppliersForCampaign(c *gin.Context) {

	header := c.GetHeader("Authorization")

	tenantId, err := utils.ExtractTokenTenantID(header)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get tenant id"})
		return
	}

	idParam := c.Param("campaign_id")
	campaignID, err := uuid.Parse(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid tenant id"})
		return
	}

	supplierIDs, err := cfg.DBQueries.GetAllSuppliersForCampaign(c, queries.GetAllSuppliersForCampaignParams{
		TenantID:   tenantId,
		CampaignID: campaignID,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list suppliers"})
		return
	}


	var suppliers []queries.Supplier
	for _, supplierID := range supplierIDs {
    	supplier, err := cfg.DBQueries.GetSupplierByID(c, queries.GetSupplierByIDParams{
        ID:       supplierID,   // Assuming supplierID contains the actual ID field
        TenantID: tenantId,
	    })
	    if err != nil {
	        // If an error occurs for a specific supplier, log the error and continue fetching other suppliers
	        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch supplier details"})
	        return
	    }
	    suppliers = append(suppliers, supplier)
	}


	c.JSON(http.StatusOK, gin.H{"suppliers": suppliers})


}





func (cfg *ApiConfig) AddSupplierToCampaign(c *gin.Context) {

	header := c.GetHeader("Authorization")

	tenantId, err := utils.ExtractTokenTenantID(header)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get tenant id"})
		return
	}


	idParam := c.Param("campaign_id")
	campaignID, err := uuid.Parse(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid campaign id"})
		return
	}


	var req struct {
		SupplierID  uuid.UUID `json:"supplier_id"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}


	_, err = cfg.DBQueries.CreateCampaignSupplierRelation(c, queries.CreateCampaignSupplierRelationParams{
		TenantID:    tenantId,
		CampaignID:  campaignID,
		SupplierID:  req.SupplierID,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create campaign", "message": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "successfully added supplier to campaign"})



}



func (cfg *ApiConfig) ListCampaignsForSupplier(c *gin.Context) {

	header := c.GetHeader("Authorization")

	tenantId, err := utils.ExtractTokenTenantID(header)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get tenant id"})
		return
	}

	// get the user id from the token
	userId, err := utils.ExtractTokenUserID(header);
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get user id", "message": err.Error()})
		return
	}


	// get supplier id from the supplier_users table
	supplier_id, err := cfg.DBQueries.GetSupplierForUser(c, userId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get supplier id"})
		return
	}

	// Get all campaigns this supplier is associated with


	campaigns, err := cfg.DBQueries.ListCampaignsForSupplier(c, queries.ListCampaignsForSupplierParams{
		TenantID:   tenantId,
		SupplierID: supplier_id,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list campaigns"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"campaigns": campaigns})


}



func (cfg *ApiConfig) GetCampaignForSupplier(c *gin.Context) {

	header := c.GetHeader("Authorization")

	tenantId, err := utils.ExtractTokenTenantID(header)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get tenant id"})
		return
	}

	// get the user id from the token
	userId, err := utils.ExtractTokenUserID(header);
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get user id"})
		return
	}

	idParam := c.Param("campaign_id")
	campaignID, err := uuid.Parse(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid campaign id"})
		return
	}


	// get supplier id from the supplier_users table
	supplier_id, err := cfg.DBQueries.GetSupplierForUser(c, userId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get user id"})
		return
	}


	campaign, err := cfg.DBQueries.GetCampaignByIDForSupplier(c, queries.GetCampaignByIDForSupplierParams{
		ID:         campaignID,
		SupplierID: supplier_id,
		TenantID:   tenantId,
	})
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "campaign not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"campaign": campaign})


}


func (cfg *ApiConfig) UpdateCampaign(c *gin.Context) {
	header := c.GetHeader("Authorization")

	tenantId, err := utils.ExtractTokenTenantID(header)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get tenant id"})
		return
	}

	idParam := c.Param("campaign_id")
	campaignID, err := uuid.Parse(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid campaign id"})
		return
	}

	var req struct {
		Name             *string     `json:"name,omitempty"`
		DeliverySchedule *string     `json:"delivery_schedule,omitempty"`
		BuyerID          uuid.UUID   `json:"buyer_id,omitempty"`
		IsActive         *bool       `json:"is_active,omitempty"`
		Description      *string     `json:"description,omitempty"`
		StartDate        *time.Time  `json:"start_date,omitempty"`
		EndDate          *time.Time  `json:"end_date,omitempty"`
		SupplierIDs      []uuid.UUID `json:"supplier_ids,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get existing campaign
	campaign, err := cfg.DBQueries.GetCampaignByID(c, queries.GetCampaignByIDParams{
		ID:       campaignID,
		TenantID: tenantId,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get campaign"})
		return
	}

	// Update only provided fields
	if req.Name != nil {
		campaign.Name = *req.Name
	}
	if req.DeliverySchedule != nil {
		campaign.DeliverySchedule = sql.NullString{
			String: *req.DeliverySchedule,
			Valid:  true,
		}
	}
	if req.BuyerID != uuid.Nil {
		campaign.BuyerID = uuid.NullUUID{
			UUID:  req.BuyerID,
			Valid: true,
		}
	}
	if req.IsActive != nil {
		campaign.IsActive = sql.NullBool{
			Bool:  *req.IsActive,
			Valid: true,
		}
	}
	if req.Description != nil {
		campaign.Description = sql.NullString{
			String: *req.Description,
			Valid:  true,
		}
	}
	if req.StartDate != nil {
		campaign.StartDate = sql.NullTime{
			Time:  *req.StartDate,
			Valid: true,
		}
	}
	if req.EndDate != nil {
		campaign.EndDate = sql.NullTime{
			Time:  *req.EndDate,
			Valid: true,
		}
	}

	// Update campaign record
	updatedCampaign, err := cfg.DBQueries.UpdateCampaign(c, queries.UpdateCampaignParams{
		ID:               campaignID,
		TenantID:         tenantId,
		Name:             campaign.Name,
		BuyerID:          campaign.BuyerID,
		DeliverySchedule: campaign.DeliverySchedule,
		IsActive:         campaign.IsActive,
		Description:      campaign.Description,
		StartDate:        campaign.StartDate,
		EndDate:          campaign.EndDate,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update campaign"})
		return
	}

	// --- Replace campaign supplier relationships ---
	if err := cfg.DBQueries.DeleteCampaignSuppliersByCampaignID(c, queries.DeleteCampaignSuppliersByCampaignIDParams{
		CampaignID: campaignID,
		TenantID:   tenantId,
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to remove old supplier relationships"})
		return
	}

	for _, supplierID := range req.SupplierIDs {
		_, err := cfg.DBQueries.CreateCampaignSupplierRelation(c, queries.CreateCampaignSupplierRelationParams{
			TenantID:   tenantId,
			CampaignID: campaignID,
			SupplierID: supplierID,
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create supplier relationship"})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"campaign": updatedCampaign})
}


