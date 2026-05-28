package main

import (
	"net/http"

	"butterfly.orx.me/core/app"
	"github.com/gin-gonic/gin"
)

func main() {
	config := &app.Config{
		Service: "neo-line",
		Router: func(r *gin.Engine) {
			r.GET("/ping", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"message": "pong"})
			})
		},
	}

	application := app.New(config)
	application.Run()
}
