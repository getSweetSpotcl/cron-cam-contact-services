# Go Boilerplate for SweetSpot

This repository contains a boilerplate Go application designed to be deployed on SweetSpot infrastructure.

## Overview

This boilerplate provides a starting point for developing Go applications that are compatible with SweetSpot's deployment environment. It includes the basic structure and configuration needed to get your application up and running quickly.

## Getting Started

### Prerequisites

- Go 1.18 or higher
- Git
- Access to SweetSpot infrastructure

### Local development

1. Make all the changes in the code.
2. Create the image with docker build <<name>> .
3. una vez que este todo listo hace push en la rama en la que estes trabajndo (feat1, fix23)
4. Hace merge en QA para poder hacer todas las pruebas necesarias 
5. Hace el release recordando hacerlo como prerelease y desde la rama QA
6. Cuando ya estes listo para pasar a prod hace el merge a main
7. Crea el release en la rama main con el tag correspondiente siguiendo semnatic versioning.

Recuerda utilizar la herramienta deploySweet para desplegar los manifiestos de kubernetes que crearan la infraestructura
