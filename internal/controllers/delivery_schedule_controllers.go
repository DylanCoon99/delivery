package controllers


import (
    //"os"
    //"log"
    "time"
    //"errors"
    "net/http"
    //"strconv"
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




func (cfg *ApiConfig) CreateDeliverySchedule(c *gin.Context) {
    header := c.GetHeader("Authorization")

    tenantId, err := utils.ExtractTokenTenantID(header)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get tenant id"})
        return
    }

    // --- Buyer ID from URL ---
    idParam := c.Param("id")
    buyerID, err := uuid.Parse(idParam)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid buyer id"})
        return
    }

    // --- Request Body ---
    var req struct {
        CronExpression string `json:"cron_expression" binding:"required"`
        Timezone       string `json:"timezone"`
        IsActive       *bool  `json:"is_active"`
    }

    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    // --- Prepare Null Types ---
    cron := sql.NullString{
        String: req.CronExpression,
        Valid:  req.CronExpression != "",
    }

    timezone := sql.NullString{
        String: req.Timezone,
        Valid:  req.Timezone != "",
    }

    // IsActive is NOT passed to SQL anymore; schedule defaults to TRUE in table.
    // Column6 is placeholder for future fields, so we set it to nil.

    scheduleID := uuid.New()

    // --- Create Schedule via SQLC ---
    schedule, err := cfg.DBQueries.CreateDeliverySchedule(c, queries.CreateDeliveryScheduleParams{
        ID:             scheduleID,
        TenantID:       tenantId,
        BuyerID:        uuid.NullUUID{UUID: buyerID, Valid: true},
        CronExpression: cron,
        Timezone:       timezone,
        Column6:        nil,            // reserved future column
        LastRunAt:      sql.NullTime{}, // initially null
        NextRunAt:      sql.NullTime{}, // computed later by scheduler
    })

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{
            "error":   "failed to create delivery schedule",
            "message": err.Error(),
        })
        return
    }

    c.JSON(http.StatusCreated, gin.H{
        "message":           "successfully created delivery schedule",
        "delivery_schedule": schedule,
        "tenant_id":         tenantId,
    })
}


func (cfg *ApiConfig) ListDeliverySchedulesByBuyer(c *gin.Context) {

    header := c.GetHeader("Authorization")

    tenantId, err := utils.ExtractTokenTenantID(header)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get tenant id"})
        return
    }

    // --- Buyer ID from URL ---
    idParam := c.Param("id")
    buyerID, err := uuid.Parse(idParam)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid buyer id"})
        return
    }

    // --- Fetch delivery schedules ---
    schedules, err := cfg.DBQueries.ListSchedulesByBuyer(
        c,
        queries.ListSchedulesByBuyerParams{
            BuyerID:  uuid.NullUUID{UUID: buyerID, Valid: true},
            TenantID: tenantId,
        },
    )
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{
            "error":   "failed to fetch delivery schedules",
            "message": err.Error(),
        })
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "tenant_id":          tenantId,
        "buyer_id":           buyerID,
        "delivery_schedules": schedules,
    })
}



func (cfg *ApiConfig) GetDeliverySchedule(c *gin.Context) {
    header := c.GetHeader("Authorization")

    tenantId, err := utils.ExtractTokenTenantID(header)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get tenant id"})
        return
    }

    // --- Schedule ID from URL ---
    idParam := c.Param("id")
    scheduleID, err := uuid.Parse(idParam)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid delivery schedule id"})
        return
    }

    // --- Build SQL params ---
    params := queries.GetDeliveryScheduleParams{
        ID:       scheduleID,
        TenantID: tenantId,
    }

    // --- Fetch delivery schedule ---
    schedule, err := cfg.DBQueries.GetDeliverySchedule(c, params)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{
            "error":   "failed to fetch delivery schedule",
            "message": err.Error(),
        })
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "tenant_id":         tenantId,
        "delivery_schedule": schedule,
    })
}



func (cfg *ApiConfig) UpdateDeliverySchedule(c *gin.Context) {
    header := c.GetHeader("Authorization")

    tenantId, err := utils.ExtractTokenTenantID(header)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get tenant id"})
        return
    }

    // --- Schedule ID from URL ---
    idParam := c.Param("id")
    scheduleID, err := uuid.Parse(idParam)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid delivery schedule id"})
        return
    }

    // --- Request Body ---
    var req struct {
        CronExpression string      `json:"cron_expression"`
        Timezone       string      `json:"timezone"`
        IsActive       *bool       `json:"is_active"`
        LastRunAt      *time.Time  `json:"last_run_at"`
        NextRunAt      *time.Time  `json:"next_run_at"`
    }

    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    // --- Prepare SQL Null types ---
    cron := sql.NullString{
        String: req.CronExpression,
        Valid:  req.CronExpression != "",
    }

    timezone := sql.NullString{
        String: req.Timezone,
        Valid:  req.Timezone != "",
    }

    isActive := sql.NullBool{Valid: false}
    if req.IsActive != nil {
        isActive = sql.NullBool{
            Bool:  *req.IsActive,
            Valid: true,
        }
    }

    lastRunAt := sql.NullTime{}
    if req.LastRunAt != nil {
        lastRunAt = sql.NullTime{Time: *req.LastRunAt, Valid: true}
    }

    nextRunAt := sql.NullTime{}
    if req.NextRunAt != nil {
        nextRunAt = sql.NullTime{Time: *req.NextRunAt, Valid: true}
    }

    // --- Build params for new SQL signature ---
    params := queries.UpdateDeliveryScheduleParams{
        ID:             scheduleID,
        CronExpression: cron,
        Timezone:       timezone,
        IsActive:       isActive,
        LastRunAt:      lastRunAt,
        NextRunAt:      nextRunAt,
        TenantID:       tenantId,   // <-- NEW REQUIRED FIELD
    }

    // --- Update delivery schedule ---
    schedule, err := cfg.DBQueries.UpdateDeliverySchedule(c, params)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{
            "error":   "failed to update delivery schedule",
            "message": err.Error(),
        })
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "message":           "successfully updated delivery schedule",
        "delivery_schedule": schedule,
        "tenant_id":         tenantId,
    })
}


func (cfg *ApiConfig) DeleteDeliverySchedule(c *gin.Context) {
    header := c.GetHeader("Authorization")

    tenantId, err := utils.ExtractTokenTenantID(header)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get tenant id"})
        return
    }

    // --- Schedule ID from URL ---
    idParam := c.Param("id")
    scheduleID, err := uuid.Parse(idParam)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid delivery schedule id"})
        return
    }

    // --- Build arguments for SQL ---
    params := queries.DeleteDeliveryScheduleParams{
        ID:       scheduleID,
        TenantID: tenantId,
    }

    // --- Execute delete ---
    err = cfg.DBQueries.DeleteDeliverySchedule(c, params)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{
            "error":   "failed to delete delivery schedule",
            "message": err.Error(),
        })
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "message":   "delivery schedule deleted",
        "schedule_id": scheduleID,
        "tenant_id": tenantId,
    })
}


func (cfg *ApiConfig) TriggerDeliverySchedule(c *gin.Context) {
    header := c.GetHeader("Authorization")

    tenantId, err := utils.ExtractTokenTenantID(header)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get tenant id"})
        return
    }

    // --- Schedule ID from URL ---
    idParam := c.Param("id")
    scheduleID, err := uuid.Parse(idParam)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid delivery schedule id"})
        return
    }

    // --- Ensure schedule belongs to tenant ---
    schedule, err := cfg.DBQueries.GetDeliverySchedule(c, queries.GetDeliveryScheduleParams{
        ID:       scheduleID,
        TenantID: tenantId,
    })
    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{
            "error":   "delivery schedule not found",
            "message": err.Error(),
        })
        return
    }

    /*
    // --- Trigger job (stub) ---
    // Replace this with your task queue, Pub/Sub, Cloud Tasks, etc.
    err = cfg.RunDeliverySchedule(schedule)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{
            "error":   "failed to trigger schedule",
            "message": err.Error(),
        })
        return
    }
    */

    c.JSON(http.StatusOK, gin.H{
        "message":           "schedule triggered",
        "delivery_schedule": schedule,
        "tenant_id":         tenantId,
    })
}


func (cfg *ApiConfig) ListDueSchedules(c *gin.Context) {
    header := c.GetHeader("Authorization")

    tenantId, err := utils.ExtractTokenTenantID(header)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get tenant id"})
        return
    }

    // --- Fetch due schedules ---
    schedules, err := cfg.DBQueries.ListDueSchedules(c, tenantId)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{
            "error":   "failed to fetch due delivery schedules",
            "message": err.Error(),
        })
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "tenant_id":         tenantId,
        "due_schedules":     schedules,
        "count":             len(schedules),
    })
}



func (cfg *ApiConfig) PauseDeliverySchedule(c *gin.Context) {


}



func (cfg *ApiConfig) ResumeDeliverySchedule(c *gin.Context) {


}

