Chirpy - Microblogging API
<img src="https://img.shields.io/badge/Go-1.18+-blue?logo=go" alt="Go">
<img src="https://img.shields.io/badge/PostgreSQL-13+-blue?logo=postgresql" alt="PostgreSQL">
<img src="https://img.shields.io/badge/JWT-Authentication-brightgreen" alt="JWT">

Chirpy is a RESTful API for a microblogging platform where users can create short posts (chirps), authenticate securely, and manage their accounts. It demonstrates modern backend development practices using Go, PostgreSQL, and JWT authentication.

Why You Should Care
This project serves as a comprehensive learning resource for:

Building RESTful APIs with Go
Implementing JWT authentication
Using database query generators (sqlc)
Designing clean, maintainable backend services
Handling real-world requirements like sorting and filtering
Perfect for developers looking to understand production-ready backend patterns.

Tools Used
Go (1.18+)
PostgreSQL (13+)
sqlc (for database query generation)
JWT (JSON Web Tokens for authentication)
Environment variables for configuration
Installation & Setup
Prerequisites
Go 1.18+
PostgreSQL
sqlc (for database code generation)
Setup Steps
Create a PostgreSQL database:
```
createdb chirpy
```

Create a .env file with required variables:
```
DB_URL=postgres://user:password@localhost:5432/chirpy?sslmode=disable
JWT_SECRET=your_strong_secret_key
POLKA_KEY=your_polka_key
```

Generate database code (if needed):
```
sqlc generate
```

Build and run the application:
```
go build -o chirpy
./chirpy
```

API Endpoints
Endpoint	Method	Description
/api/chirps	GET	Get chirps (with sort=asc/desc param)
/api/chirps	POST	Create new chirp (requires auth)
/api/login	POST	Authenticate and get JWT token
/api/users	POST	Create new user
/api/users	PUT	Update user profile
/api/polka/webhooks	POST	Handle Polka webhook events
Running Tests

The project includes CLI tests that can be run with:
```
go test ./...
```
Example Usage
```
# Create a chirp
curl -X POST http://localhost:8080/api/chirps \
  -H "Authorization: Bearer <token>" \
  -d '{"body": "Hello, world!"}'

# Get chirps sorted by creation time
curl "http://localhost:8080/api/chirps?sort=desc"
```
Note: This project is designed for educational purposes and demonstrates best practices in backend development. For production use, additional security measures and error handling would be required.