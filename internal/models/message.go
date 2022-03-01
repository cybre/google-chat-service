package models

type Message struct {
	Recipient string
	Body      string
}

type Messages []Message
