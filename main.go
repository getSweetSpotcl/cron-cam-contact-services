package main

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Estructura para los datos de respuesta
type Response struct {
	Message string `json:"message"`
	Status  string `json:"status"`
}

func helloHandler(c *gin.Context) {
	response := Response{
		Message: "Hola Mundo",
		Status:  "success",
	}
	c.JSON(http.StatusOK, response)
}

// Manejador para obtener datos
func getDataHandler(c *gin.Context) {
	// Ejemplo de parámetro en la URL
	id := c.Param("id")

	response := Response{
		Message: "Obteniendo datos para ID: " + id,
		Status:  "success",
	}
	c.JSON(http.StatusOK, response)
}

// Manejador para crear datos
func createDataHandler(c *gin.Context) {
	var requestData struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}

	if err := c.ShouldBindJSON(&requestData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	response := Response{
		Message: "Datos creados para: " + requestData.Name,
		Status:  "success",
	}
	c.JSON(http.StatusCreated, response)
}

func main() {
	// Inicializar el router de Gin
	router := gin.Default()

	// Definir rutas
	router.GET("/", helloHandler)
	router.GET("/api/data/:id", getDataHandler)
	router.POST("/api/data", createDataHandler)

	// Grupo de rutas con middleware de autenticación
	authorized := router.Group("/admin")
	authorized.Use(authMiddleware())
	{
		authorized.GET("/data", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"message": "Datos administrativos"})
		})
	}

	log.Println("Servidor API escuchando en el puerto 8080...")
	if err := router.Run(":8080"); err != nil {
		log.Fatalf("Error al iniciar el servidor: %v", err)
	}
}

// Middleware de ejemplo para autenticación
func authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.GetHeader("Authorization")
		if token != "mi-token-secreto" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "No autorizado"})
			return
		}
		c.Next()
	}
}
