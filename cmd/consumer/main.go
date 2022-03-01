package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/cybre/google-chat-service/internal/models"
	"github.com/streadway/amqp"
	"github.com/tebeka/selenium"
)

func main() {
	seleniumServerHost, ok := os.LookupEnv("SELENIUM_SERVER_HOST")
	if !ok {
		log.Fatal("Could not find SELENIUM_SERVER_HOST in env")
	}

	// Connect to selenium server instance
	caps := selenium.Capabilities{"browserName": "chrome"}
	wd, err := selenium.NewRemote(caps, fmt.Sprintf("http://%s", seleniumServerHost))
	if err != nil {
		log.Fatalf("Could not connect to selenium server: %v", err)
	}
	defer wd.Quit()

	wd.SetImplicitWaitTimeout(30 * time.Second)

	// Navigate to Google Chat
	if err := wd.Get("https://chat.google.com"); err != nil {
		log.Fatalf("Could not navigate to Google Chat: %v", err)
	}

	// Get a reference to the email text box.
	emailInput, err := wd.FindElement(selenium.ByCSSSelector, "input[name=identifier]")
	if err != nil {
		log.Fatalf("Could not find email input: %v", err)
	}
	email, ok := os.LookupEnv("USER_EMAIL")
	if !ok {
		log.Fatal("Could not find USER_EMAIL in env")
	}
	if err = emailInput.SendKeys(email); err != nil {
		log.Fatalf("Could not enter email: %v", err)
	}
	if err = emailInput.SendKeys(selenium.EnterKey); err != nil {
		log.Fatalf("Could not submit email: %v", err)
	}

	// Wait for password input to appear
	var passwordInput selenium.WebElement
	if err = wd.WaitWithTimeoutAndInterval(func(wd selenium.WebDriver) (bool, error) {
		passwordInput, err = wd.FindElement(selenium.ByCSSSelector, "input[name=password]")
		if err != nil {
			return false, err
		}
		displayed, err := passwordInput.IsDisplayed()
		if err != nil {
			return false, err
		}
		return displayed, nil
	}, 30*time.Second, 1*time.Second); err != nil {
		log.Fatalf("Error waiting for password input: %v", err)
	}

	password, ok := os.LookupEnv("USER_PASSWORD")
	if !ok {
		log.Fatal("Could not find USER_PASSWORD in env")
	}
	if err = passwordInput.SendKeys(password); err != nil {
		log.Fatalf("Could not enter password: %v", err)
	}
	if err = passwordInput.SendKeys(selenium.EnterKey); err != nil {
		log.Fatalf("Could not submit password: %v", err)
	}

	// Wait for navigation to occur and search input to be available
	var searchInput selenium.WebElement
	if err = wd.WaitWithTimeoutAndInterval(func(wd selenium.WebDriver) (bool, error) {
		searchInput, err = wd.FindElement(selenium.ByCSSSelector, "input[name=q]")
		if err != nil {
			return false, err
		}
		displayed, err := searchInput.IsDisplayed()
		if err != nil {
			return false, err
		}
		return displayed, nil
	}, 60*time.Second, 2*time.Second); err != nil {
		log.Fatalf("Error waiting for search input: %v", err)
	}

	url, err := wd.CurrentURL()
	if err != nil {
		log.Fatalf("Could not get current url: %v", err)
	}
	fmt.Printf("url: %v\n", url)

	// Connect to RabbitMQ
	rabbitMQHost, ok := os.LookupEnv("RABBITMQ_HOST")
	if !ok {
		log.Fatal("Could not find RABBITMQ_HOST in env")
	}
	conn, err := amqp.Dial(fmt.Sprintf("amqp://guest:guest@%s/", rabbitMQHost))
	if err != nil {
		log.Fatalf("Could not connect to RabbitMQ: %v", err)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		log.Fatalf("Failed to open a channel: %v", err)
	}
	defer ch.Close()

	msgs, err := ch.Consume(
		"messages", // queue
		"",         // consumer
		true,       // auto-ack
		false,      // exclusive
		false,      // no-local
		false,      // no-wait
		nil,        // args
	)

	for msg := range msgs {
		var messages models.Messages
		if err := json.Unmarshal(msg.Body, &messages); err != nil {
			log.Printf("Failed to decode message: %v", err)
			continue
		}

		for _, message := range messages {
			if err = searchInput.Clear(); err != nil {
				log.Printf("Failed to clear search input: %v", err)
				continue
			}
			if err = searchInput.SendKeys(message.Recipient); err != nil {
				log.Printf("Failed to enter search query: %v", err)
				continue
			}

			// Wait for search results
			var searchResult selenium.WebElement
			if err = wd.WaitWithTimeout(func(wd selenium.WebDriver) (bool, error) {
				searchResult, err = wd.FindElement(selenium.ByCSSSelector, "tr[role=option] div[role=button]")
				if err != nil {
					return false, err
				}
				displayed, err := searchResult.IsDisplayed()
				if err != nil {
					return false, err
				}
				if !displayed {
					return false, nil
				}
				textContext, err := searchResult.Text()
				if err != nil {
					return false, nil
				}
				return strings.HasSuffix(textContext, message.Recipient), nil
			}, 10*time.Second); err != nil {
				log.Printf("Error waiting for search result: %v", err)
				continue
			}

			if err := searchResult.Click(); err != nil {
				log.Printf("Could not click search result: %v", err)
				continue
			}

			time.Sleep(1 * time.Second)

			// Wait for chat frame
			frame, err := wd.FindElement(selenium.ByCSSSelector, "iframe[title='Chat content']")
			if err != nil {
				log.Printf("Could not find chat frame: %v", err)
				continue
			}
			if err = wd.SwitchFrame(frame); err != nil {
				log.Printf("Failed to switch to chat frame: %v", err)
				continue
			}

			textBox, err := wd.FindElement(selenium.ByCSSSelector, "div[role=textbox]")
			if err != nil {
				log.Printf("Could not find message inptu: %v", err)
				continue
			}
			if err = textBox.SendKeys(message.Body); err != nil {
				log.Printf("Failed to enter message body: %v", err)
				continue
			}
			if err = textBox.SendKeys(selenium.EnterKey); err != nil {
				log.Printf("Failed to submit message: %v", err)
				continue
			}
			if err = wd.SwitchFrame(nil); err != nil {
				log.Printf("Failed to switch frame to top-level: %v", err)
				continue
			}
		}
	}
}
