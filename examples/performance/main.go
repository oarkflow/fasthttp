package main

import (
	"log"
	"strconv"

	"github.com/oarkflow/fh"
)

type User struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type Users []User

func (u Users) AppendJSON(dst []byte) ([]byte, error) {
	dst = append(dst, '[')
	for i, user := range u {
		if i > 0 {
			dst = append(dst, ',')
		}
		dst = append(dst, `{"id":`...)
		dst = strconv.AppendInt(dst, int64(user.ID), 10)
		dst = append(dst, `,"name":"`...)
		dst = appendJSONString(dst, user.Name)
		dst = append(dst, `"}`...)
	}
	dst = append(dst, ']')
	return dst, nil
}

func appendJSONString(dst []byte, s string) []byte {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\', '"':
			dst = append(dst, '\\', s[i])
		case '\n':
			dst = append(dst, '\\', 'n')
		case '\r':
			dst = append(dst, '\\', 'r')
		case '\t':
			dst = append(dst, '\\', 't')
		default:
			dst = append(dst, s[i])
		}
	}
	return dst
}

func main() {
	users := make(Users, 100)
	for i := range users {
		users[i] = User{ID: i + 1, Name: "User " + strconv.Itoa(i+1)}
	}

	app := fh.New()
	app.Get("/plaintext", func(c *fh.Ctx) error { return c.SendString("Hello, World!") })
	app.Get("/json", func(c *fh.Ctx) error { return c.JSONString(`{"message":"Hello, World!"}`) })
	app.Get("/users/:id", func(c *fh.Ctx) error { return c.JSONString(`{"name":"User ` + c.Param("id") + `"}`) })
	app.Get("/search", func(c *fh.Ctx) error { return c.JSONString(`{"query":"` + c.Query("q") + `"}`) })
	app.Post("/echo", func(c *fh.Ctx) error { return c.EchoJSON() })
	app.Get("/users", func(c *fh.Ctx) error { return c.JSON(users) })

	log.Fatal(app.Listen(":3000"))
}
