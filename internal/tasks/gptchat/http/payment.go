package http

import (
	"net/http"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v6"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
	"github.com/stripe/stripe-go/v76"
	"github.com/stripe/stripe-go/v76/paymentintent"

	"github.com/Laisky/go-ramjet/library/web"
)

type paymentItem struct {
	// id string
}

type paymentRequest struct {
	Items []paymentItem `json:"items"`
}

// PaymentHandler creates a Stripe PaymentIntent.
func PaymentHandler(c *gin.Context) {
	logger := gmw.GetLogger(c)
	if c.Request.Method != http.MethodPost {
		web.AbortErr(c, errors.New("only support POST method"))
		return
	}

	req := new(paymentRequest)
	if err := c.BindJSON(req); web.AbortErr(c, err) {
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
	if web.AbortErr(c, err) {
		return
	}

	logger.Info("create payment intent",
		zap.Int64("amount", amount),
		zap.String("client", pi.ID))
	c.JSON(http.StatusOK, gin.H{
		"clientSecret": pi.ClientSecret,
	})
}
