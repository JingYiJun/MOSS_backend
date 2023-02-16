package account

import "github.com/gofiber/fiber/v2"

func RegisterRoutes(routes fiber.Router) {
	// token
	routes.Post("/login", Login)
	routes.Get("/logout", Logout)
	routes.Post("/refresh", Refresh)

	// account management
	routes.Get("/verify/email", VerifyWithEmail)
	routes.Get("/verify/phone", VerifyWithPhone)
	routes.Post("/register", Register)
	routes.Put("/register", ChangePassword)
	routes.Delete("/users/me", DeleteUser)

	// user info
	routes.Get("/users/me", GetCurrentUser)
	routes.Put("/users/me", ModifyUser)
}
