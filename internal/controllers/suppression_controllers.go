package controllers


import (
	//"os"
	"fmt"
	"log"
	//"time"
	"strings"
	"net/http"
	"encoding/csv"
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



func (cfg *ApiConfig) GetSuppressionList(c *gin.Context) {

	header := c.GetHeader("Authorization")

	tenantId, err := utils.ExtractTokenTenantID(header)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get tenant id"})
		return
	}

	campaignIDParam := c.Param("campaign_id")
	campaignId, err := uuid.Parse(campaignIDParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid campaign id"})
		return
	}


	suppression_list, err := cfg.DBQueries.GetSuppressionListForCampaign(c, queries.GetSuppressionListForCampaignParams{
		ID:       campaignId,
		TenantID: tenantId,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get suppression list"})
		return
	}

    // Fetch suppression entries for this list
    entries, err := cfg.DBQueries.GetSuppressionEntriesByListID(c, suppression_list)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch entries", "details": err.Error()})
        return
    }

    // need to decrypt the PII for each lead
	for i := range entries {
		if entries[i].EmailHash.Valid {
			decryptedEmail, err := utils.DecryptString(entries[i].EmailHash.String)
			if err == nil {
				entries[i].EmailHash.String = decryptedEmail
			}
		}
		if entries[i].PhoneHash.Valid {
			decryptedPhone, err := utils.DecryptString(entries[i].PhoneHash.String)
			if err == nil {
				entries[i].PhoneHash.String = decryptedPhone
			}
		}
	}

    c.JSON(http.StatusOK, gin.H{"id": suppression_list, "list": entries})


}



func (cfg *ApiConfig) GetSuppressionAttributes(c *gin.Context) {
	header := c.GetHeader("Authorization")

	tenantId, err := utils.ExtractTokenTenantID(header)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get tenant id"})
		return
	}

	campaignIDParam := c.Param("id")
	campaignId, err := uuid.Parse(campaignIDParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid campaign id"})
		return
	}

	// Get suppression list for this campaign
	suppressionList, err := cfg.DBQueries.GetSuppressionListForCampaign(c, queries.GetSuppressionListForCampaignParams{
		ID:       campaignId,
		TenantID: tenantId,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get suppression list"})
		return
	}

	// --- Fetch all attribute types ---
	companies, err := cfg.DBQueries.GetSuppressionCompaniesByListID(c, suppressionList)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get suppression companies", "details": err.Error()})
		return
	}

	regions, err := cfg.DBQueries.GetSuppressionRegionsByListID(c, suppressionList)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get suppression regions", "details": err.Error()})
		return
	}

	states, err := cfg.DBQueries.GetSuppressionStatesByListID(c, suppressionList)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get suppression states", "details": err.Error()})
		return
	}

	employeeSizes, err := cfg.DBQueries.GetSuppressionEmployeeSizesByListID(c, suppressionList)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get suppression employee sizes", "details": err.Error()})
		return
	}

	revenueSizes, err := cfg.DBQueries.GetSuppressionRevenueSizesByListID(c, suppressionList)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get suppression revenue sizes", "details": err.Error()})
		return
	}

	// --- Construct the unified response ---
	response := gin.H{
		"companies":        companies,
		"regions":          regions,
		"states":           states,
		"employee_size_ranges": employeeSizes,
		"revenue_size_ranges":  revenueSizes,
	}

	c.JSON(http.StatusOK, gin.H{
		"id":         suppressionList,
		"attributes": response,
	})
}



func (cfg *ApiConfig) CreateSuppressionList(c *gin.Context) {

	header := c.GetHeader("Authorization")

	tenantId, err := utils.ExtractTokenTenantID(header)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get tenant id"})
		return
	}

	campaignIDParam := c.Param("campaign_id")
	campaignId, err := uuid.Parse(campaignIDParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid campaign id"})
		return
	}


	var req struct {
		Name         string `json:"name" binding:"required"`
		Description  string `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	description := sql.NullString{
		String: req.Description,
		Valid:  req.Description != "",
	}


	suppression_list, err := cfg.DBQueries.CreateSuppressionList(c, queries.CreateSuppressionListParams{
		TenantID:     tenantId,
		Name:         req.Name,
		Description:  description,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create suppression list", "message": err.Error()})
		return
	}



	// Update the campaign with the suppression_list_id
	err = cfg.DBQueries.AddSuppressionListToCampaign(c, queries.AddSuppressionListToCampaignParams{
		ID:                campaignId,
		TenantID:          tenantId,
		SuppressionListID: suppression_list.ID,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to add suppression list to campaign", "message": err.Error()})
		return
	}




	c.JSON(http.StatusCreated, gin.H{"suppression_list": suppression_list})
}



func (cfg *ApiConfig) DeleteSuppressionList(c *gin.Context) {

	header := c.GetHeader("Authorization")

	tenantId, err := utils.ExtractTokenTenantID(header)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get tenant id"})
		return
	}

	suppressionListIDParam := c.Param("suppression_list_id")
	suppressionListId, err := uuid.Parse(suppressionListIDParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid suppression id"})
		return
	}

	


	err = cfg.DBQueries.DeleteSuppressionList(c, queries.DeleteSuppressionListParams{
		TenantID: tenantId,
		ID:       suppressionListId,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete suppression list"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "suppression list deleted"})
}



func (cfg *ApiConfig) BulkUploadSuppressionEntries(c *gin.Context) {

	
	header := c.GetHeader("Authorization")

	
	tenantId, err := utils.ExtractTokenTenantID(header)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get tenant id"})
		return
	}

	campaignIDParam := c.Param("campaign_id")
	campaignId, err := uuid.Parse(campaignIDParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid campaign id"})
		return
	}

	// Get the suppression list for this campaign
	suppression_list_id, err := cfg.DBQueries.GetSuppressionListForCampaign(c, queries.GetSuppressionListForCampaignParams{
		ID:       campaignId,
		TenantID: tenantId,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get suppression list"})
		return
	}


	// Use this suppression_list_id to create entries for each row in the csv
	

	// Retrieve the uploaded file from the form data
	fileHeader, err := c.FormFile("csvFile") // "csvFile" is the name of the form field
	if err != nil {
		c.String(http.StatusBadRequest, fmt.Sprintf("Error retrieving file: %s", err.Error()))
		return
	}

	log.Printf("Uploaded file: %s", fileHeader.Filename)

	// Open the uploaded file
	file, err := fileHeader.Open()
	if err != nil {
		c.String(http.StatusInternalServerError, fmt.Sprintf("Error opening file: %s", err.Error()))
		return
	}
	defer file.Close()

	// Create a new CSV reader
	reader := csv.NewReader(file)

	// Read all records from the CSV file
	records, err := reader.ReadAll()
	if err != nil {
		c.String(http.StatusInternalServerError, fmt.Sprintf("Error reading CSV: %s", err.Error()))
		return
	}


	// Process the CSV data (for testing purposes; just print)
	for i, record := range records {
		// exclude header
		if i != 0 {

			// hash the email
			emailStr, _ := utils.EncryptString(record[0])
			emailHash := sql.NullString{
				String: emailStr,
				Valid:  emailStr != "",
			}

			// hash the phone
			phoneStr, _ := utils.EncryptString(record[1])
			phoneHash := sql.NullString{
				String: phoneStr,
				Valid:  phoneStr != "",
			}

			// for each entry add a suppression entry to the database
			cfg.DBQueries.AddSuppressionEntry(c, queries.AddSuppressionEntryParams{
				SuppressionListID: suppression_list_id,
				EmailHash:         emailHash,
				PhoneHash:         phoneHash,
			})
			
		}
	}

	c.String(http.StatusOK, fmt.Sprintf("File '%s' uploaded and processed successfully!", fileHeader.Filename))

}








// DownloadSuppressionListHandler handles CSV download for a suppression list
func (cfg *ApiConfig) DownloadSuppressionList(c *gin.Context) {

	header := c.GetHeader("Authorization")
	
	tenantId, err := utils.ExtractTokenTenantID(header)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get tenant id"})
		return
	}


    listIDParam := c.Param("id")
    listID, err := uuid.Parse(listIDParam)

    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid suppression list ID"})
        return
    }

    // Fetch the suppression list
    list, err := cfg.DBQueries.GetSuppressionListByID(c, queries.GetSuppressionListByIDParams{
    	TenantID: tenantId,
    	ID:       listID,
    })
    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "suppression list not found"})
        return
    }

    // Fetch suppression entries for this list
    entries, err := cfg.DBQueries.GetSuppressionEntriesByListID(c, listID)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch entries", "details": err.Error()})
        return
    }

    // Set CSV headers for download
    c.Header("Content-Disposition", "attachment; filename="+list.Name+".csv")
    c.Header("Content-Type", "text/csv")

    writer := csv.NewWriter(c.Writer)
    defer writer.Flush()

    // Write CSV header
    writer.Write([]string{"Email", "Phone"}) // adjust columns as needed

    // Write each suppression entry
    for _, e := range entries {
    	decryptedEmail, _ := utils.DecryptString(e.EmailHash.String)
    	decryptedPhone, _ := utils.DecryptString(e.PhoneHash.String)
        writer.Write([]string{
            decryptedEmail,
            decryptedPhone,
        })
    }
}


// DownloadSuppressionListHandler handles CSV download for a suppression list
func (cfg *ApiConfig) ListSuppressionListEntries(c *gin.Context) {



    listIDParam := c.Param("id")
    listID, err := uuid.Parse(listIDParam)

    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid suppression list ID"})
        return
    }

    // Fetch suppression entries for this list
    entries, err := cfg.DBQueries.GetSuppressionEntriesByListID(c, listID)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch entries", "details": err.Error()})
        return
    }

    // need to decrypt the PII for each lead
	for i := range entries {
		if entries[i].EmailHash.Valid {
			decryptedEmail, err := utils.DecryptString(entries[i].EmailHash.String)
			if err == nil {
				entries[i].EmailHash.String = decryptedEmail
			}
		}
		if entries[i].PhoneHash.Valid {
			decryptedPhone, err := utils.DecryptString(entries[i].PhoneHash.String)
			if err == nil {
				entries[i].PhoneHash.String = decryptedPhone
			}
		}
	}

    c.JSON(http.StatusOK, gin.H{"entries": entries})

}




// Bulk upload suppression attributes
func (cfg *ApiConfig) BulkUploadSuppressionAttributes(c *gin.Context) {
	header := c.GetHeader("Authorization")

	tenantId, err := utils.ExtractTokenTenantID(header)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get tenant id"})
		return
	}

	campaignIDParam := c.Param("campaign_id")
	campaignId, err := uuid.Parse(campaignIDParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid campaign id"})
		return
	}

	// Get suppression list for this campaign
	suppressionListID, err := cfg.DBQueries.GetSuppressionListForCampaign(c, queries.GetSuppressionListForCampaignParams{
		ID:       campaignId,
		TenantID: tenantId,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get suppression list"})
		return
	}

	// Get uploaded CSV file
	fileHeader, err := c.FormFile("csvFile")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Error retrieving file: %s", err.Error())})
		return
	}

	file, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Error opening file: %s", err.Error())})
		return
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Error reading CSV: %s", err.Error())})
		return
	}

	if len(records) < 2 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "CSV must include a header and at least one data row"})
		return
	}

	// Get header row
	headerRow := records[0]
	columnIndexes := map[string]int{}

	for i, col := range headerRow {
		columnIndexes[strings.ToLower(strings.TrimSpace(col))] = i
	}

	added := 0
	for i, record := range records {
		if i == 0 {
			continue // skip header
		}

		// COMPANY NAME
		if idx, ok := columnIndexes["company"]; ok && record[idx] != "" {
			_, err := cfg.DBQueries.AddSuppressionCompany(c, queries.AddSuppressionCompanyParams{
				SuppressionListID: suppressionListID,
				CompanyName:       record[idx],
			})
			if err == nil {
				added++
			} else {
				log.Printf("Error: %v", err.Error())
			}
		} else {
			log.Printf("Failed to get company")
		}

		// REGION
		if idx, ok := columnIndexes["region"]; ok && record[idx] != "" {
			_, err := cfg.DBQueries.AddSuppressionRegion(c, queries.AddSuppressionRegionParams{
				SuppressionListID: suppressionListID,
				RegionName:        record[idx],
			})
			if err == nil {
				added++
			} else {
				log.Printf("Error: %v", err.Error())
			}
		} else {
			log.Printf("Failed to get region")
		}

		// STATE
		if idx, ok := columnIndexes["state"]; ok && record[idx] != "" {
			_, err := cfg.DBQueries.AddSuppressionState(c, queries.AddSuppressionStateParams{
				SuppressionListID: suppressionListID,
				StateName:         record[idx],
			})
			if err == nil {
				added++
			} else {
				log.Printf("Error: %v", err.Error())
			}
		} else {
			log.Printf("Failed to get state")
		}

		// EMPLOYEE SIZE
		if idx, ok := columnIndexes["employee_size"]; ok && record[idx] != "" {
			_, err := cfg.DBQueries.AddSuppressionEmployeeSize(c, queries.AddSuppressionEmployeeSizeParams{
				SuppressionListID: suppressionListID,
				SizeRange:         record[idx],
			})
			if err == nil {
				added++
			} else {
				log.Printf("Error: %v", err.Error())
			}
		} else {
			log.Printf("Failed to get employee_size")
		}

		// REVENUE SIZE
		if idx, ok := columnIndexes["revenue_size"]; ok && record[idx] != "" {
			_, err := cfg.DBQueries.AddSuppressionRevenueSize(c, queries.AddSuppressionRevenueSizeParams{
				SuppressionListID: suppressionListID,
				RevenueRange:      record[idx],
			})
			if err == nil {
				added++
			} else {
				log.Printf("Error: %v", err.Error())
			}
		} else {
			log.Printf("Failed to get revenue_size")
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"message": fmt.Sprintf("Successfully uploaded %d suppression attributes from '%s'", added, fileHeader.Filename),
	})
}


