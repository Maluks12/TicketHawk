package utils

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

func InternalError(c *gin.Context, err error, msg string) {
	log.Error().Err(err).Str("request_id", c.GetString("request_id")).Msg(msg)
	c.JSON(http.StatusInternalServerError, gin.H{"error": "An unexpected error occurred"})
}
