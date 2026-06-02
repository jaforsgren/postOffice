package workflow

import "postOffice/internal/postman"

func buildTestCollection() *postman.Collection {
	return &postman.Collection{
		Info: postman.Info{Name: "Test Collection"},
		Items: []postman.Item{
			{
				Name: "Auth",
				Items: []postman.Item{
					{
						Name: "Login",
						Request: &postman.Request{
							Method: "POST",
							URL:    postman.URL{Raw: "https://example.com/login"},
						},
					},
				},
			},
			{
				Name: "Users",
				Items: []postman.Item{
					{
						Name: "List",
						Request: &postman.Request{
							Method: "GET",
							URL:    postman.URL{Raw: "https://example.com/users"},
						},
					},
				},
			},
		},
	}
}
