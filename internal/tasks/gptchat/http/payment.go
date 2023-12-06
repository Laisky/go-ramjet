package http

import (
	"net/http"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v5"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
	"github.com/stripe/stripe-go/v76"
	"github.com/stripe/stripe-go/v76/paymentintent"

	ijs "github.com/Laisky/go-ramjet/internal/tasks/gptchat/templates/js"
	ipages "github.com/Laisky/go-ramjet/internal/tasks/gptchat/templates/pages"
	icss "github.com/Laisky/go-ramjet/internal/tasks/gptchat/templates/scss"
)

func PaymentStaticHandler(c *gin.Context) {
	switch c.Param("ext") {
	case "index.html":
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(ipages.Payment))
	case "css":
		c.Data(http.StatusOK, "text/css; charset=utf-8", []byte(icss.Payment))
	case "js":
		c.Data(http.StatusOK, "application/javascript; charset=utf-8", []byte(ijs.Payment))
	default:
		AbortErr(c, errors.New("only support html/css/js"))
	}
}

type paymentItem struct {
	id string
}

type paymentRequest struct {
	Items []paymentItem `json:"items"`
}

func PaymentHandler(c *gin.Context) {
	logger := gmw.GetLogger(c)
	if c.Request.Method != http.MethodPost {
		AbortErr(c, errors.New("only support POST method"))
		return
	}

	req := new(paymentRequest)
	if err := c.BindJSON(req); AbortErr(c, err) {
		return
	}

	var amount int64
	for range req.Items {
		amount += 1000
	}

	// Create a PaymentIntent with amount and currency
	params := &stripe.PaymentIntentParams{
		Amount:   stripe.Int64(amount),
		Currency: stripe.String(string(stripe.CurrencyCNY)),
		AutomaticPaymentMethods: &stripe.PaymentIntentAutomaticPaymentMethodsParams{
			Enabled: stripe.Bool(true),
		},
	}

	pi, err := paymentintent.New(params)
	if AbortErr(c, err) {
		return
	}

	logger.Info("create payment intent",
		zap.Int64("amount", amount),
		zap.String("client", pi.ID))
	c.JSON(http.StatusOK, gin.H{
		"clientSecret": pi.ClientSecret,
	})
}
