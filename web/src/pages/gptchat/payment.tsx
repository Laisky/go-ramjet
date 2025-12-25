import {
  Elements,
  PaymentElement,
  useElements,
  useStripe,
} from '@stripe/react-stripe-js'
import type { StripeElementsOptions } from '@stripe/stripe-js'
import { loadStripe } from '@stripe/stripe-js'
import { useEffect, useState } from 'react'
import { useTheme } from 'next-themes'

import { Button } from '@/components/ui/button'
import { API_BASE } from '@/utils/api'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'

// Initialize Stripe outside of component to avoid recreating it on renders
// Replace with your actual publishable key
const stripePromise = loadStripe('pk_test_7IZFlTmtk79XqebnMocipBb8')

type PaymentIntentResponse = {
  clientSecret?: string
}

function CheckoutForm({ clientSecret }: { clientSecret: string }) {
  const stripe = useStripe()
  const elements = useElements()

  const [message, setMessage] = useState<string | null>(null)
  const [isLoading, setIsLoading] = useState(false)

  useEffect(() => {
    if (!stripe || !clientSecret) {
      return
    }

    stripe.retrievePaymentIntent(clientSecret).then(({ paymentIntent }) => {
      switch (paymentIntent?.status) {
        case 'succeeded':
          setMessage('Payment succeeded!')
          break
        case 'processing':
          setMessage('Your payment is processing.')
          break
        case 'requires_payment_method':
          // Redirected back or just loaded
          // setMessage('Please enter your payment details.')
          break
        default:
          // setMessage('Something went wrong.')
          break
      }
    })
  }, [stripe, clientSecret])

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()

    if (!stripe || !elements) {
      return
    }

    setIsLoading(true)

    const { error } = await stripe.confirmPayment({
      elements,
      confirmParams: {
        // Redirect to same page for demo purposes, or a dedicated success page
        return_url: window.location.href,
      },
    })

    if (error.type === 'card_error' || error.type === 'validation_error') {
      setMessage(error.message || 'An unexpected error occurred.')
    } else {
      setMessage('An unexpected error occurred.')
    }

    setIsLoading(false)
  }

  return (
    <form id="payment-form" onSubmit={handleSubmit} className="space-y-6">
      <PaymentElement id="payment-element" />

      <Button
        type="submit"
        disabled={isLoading || !stripe || !elements}
        className="w-full"
      >
        {isLoading ? 'Processing...' : 'Pay now'}
      </Button>

      {message && (
        <div id="payment-message" className="text-sm text-destructive">
          {message}
        </div>
      )}
    </form>
  )
}

/**
 * GPTChatPaymentPage provides a payment intent creator and Stripe Checkout form.
 */
export function GPTChatPaymentPage() {
  const [isLoading, setIsLoading] = useState(false)
  const [clientSecret, setClientSecret] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  // Check for client secret in URL (redirect from Stripe)
  useEffect(() => {
    const secret = new URLSearchParams(window.location.search).get(
      'payment_intent_client_secret',
    )
    if (secret) {
      setClientSecret(secret)
    }
  }, [])

  async function createIntent() {
    setIsLoading(true)
    setError(null)
    setClientSecret(null)

    try {
      const resp = await fetch(`${API_BASE}/create-payment-intent`, {
        method: 'POST',
        headers: {
          'content-type': 'application/json',
          accept: 'application/json',
        },
        body: JSON.stringify({ items: [{ id: 'xl-tshirt' }] }), // Matching legacy payload
      })

      if (!resp.ok) {
        const text = await resp.text()
        throw new Error(text || `request failed: ${resp.status}`)
      }

      const json = (await resp.json()) as PaymentIntentResponse
      if (!json.clientSecret) {
        throw new Error('missing clientSecret in response')
      }

      setClientSecret(json.clientSecret)
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e)
      setError(msg)
    } finally {
      setIsLoading(false)
    }
  }

  const { resolvedTheme } = useTheme()
  const options: StripeElementsOptions = {
    clientSecret: clientSecret || '',
    appearance: {
      theme: resolvedTheme === 'dark' ? 'night' : 'stripe',
    },
  }

  return (
    <div className="mx-auto max-w-lg space-y-6 py-10">
      <div className="space-y-1 text-center">
        <h1 className="text-2xl font-semibold">GPT Chat Payment</h1>
        <p className="text-sm text-muted-foreground">
          Secure payment integration via Stripe
        </p>
      </div>

      {!clientSecret ? (
        <Card>
          <CardHeader>
            <CardTitle>Start Payment</CardTitle>
            <CardDescription>
              Create a new payment intent to proceed.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <div className="flex flex-col gap-4">
              <p className="text-sm text-muted-foreground">
                Current pricing: 1000 CNY per item.
              </p>
              <Button
                onClick={createIntent}
                disabled={isLoading}
                className="w-full"
              >
                {isLoading ? 'Initializing...' : 'Start Payment'}
              </Button>
            </div>
            {error && <p className="mt-4 text-sm text-destructive">{error}</p>}
          </CardContent>
        </Card>
      ) : (
        <Card>
          <CardHeader>
            <CardTitle>Checkout</CardTitle>
          </CardHeader>
          <CardContent>
            {clientSecret && (
              <Elements options={options} stripe={stripePromise}>
                <CheckoutForm clientSecret={clientSecret} />
              </Elements>
            )}
          </CardContent>
        </Card>
      )}
    </div>
  )
}
