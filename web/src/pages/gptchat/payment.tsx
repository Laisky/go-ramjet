import { useState } from 'react'

import { Button } from '@/components/ui/button'
import { Card, CardDescription, CardTitle } from '@/components/ui/card'

type PaymentIntentResponse = {
  clientSecret?: string
}

/**
 * GPTChatPaymentPage provides a minimal payment intent creator backed by /gptchat/create-payment-intent.
 */
export function GPTChatPaymentPage() {
  const [isLoading, setIsLoading] = useState(false)
  const [clientSecret, setClientSecret] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  async function createIntent() {
    setIsLoading(true)
    setError(null)
    setClientSecret(null)

    try {
      const resp = await fetch('/gptchat/create-payment-intent', {
        method: 'POST',
        headers: {
          'content-type': 'application/json',
          accept: 'application/json',
        },
        body: JSON.stringify({ items: [{}] }),
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

  return (
    <div className="space-y-4">
      <div className="space-y-1">
        <h1 className="text-2xl font-semibold">GPT Chat Payment</h1>
        <p className="text-sm text-black/70 dark:text-white/70">
          Creates a Stripe PaymentIntent via backend endpoint: /gptchat/create-payment-intent
        </p>
      </div>

      <Card>
        <CardTitle>Create intent</CardTitle>
        <CardDescription className="mt-1">Current backend pricing: each item = 1000 CNY.</CardDescription>
        <div className="mt-3 flex items-center justify-end">
          <Button onClick={createIntent} disabled={isLoading}>
            {isLoading ? 'Creating...' : 'Create PaymentIntent'}
          </Button>
        </div>
      </Card>

      {clientSecret ? (
        <Card>
          <CardTitle>clientSecret</CardTitle>
          <pre className="mt-2 whitespace-pre-wrap break-all text-sm">{clientSecret}</pre>
        </Card>
      ) : null}

      {error ? (
        <Card>
          <CardTitle>Error</CardTitle>
          <CardDescription className="mt-1 break-words">{error}</CardDescription>
        </Card>
      ) : null}
    </div>
  )
}
