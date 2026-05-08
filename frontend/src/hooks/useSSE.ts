import { useEffect, useRef } from 'react'

export function useSSE(onEvent: (event: { type: string; payload: unknown }) => void) {
  const onEventRef = useRef(onEvent)
  onEventRef.current = onEvent

  useEffect(() => {
    let es: EventSource | null = null
    let retryTimeout: ReturnType<typeof setTimeout> | null = null
    let retryDelay = 1000
    let stopped = false

    function connect() {
      const token = localStorage.getItem('kubepilot_token')
      const url = token
        ? `/api/v1/events?token=${encodeURIComponent(token)}`
        : '/api/v1/events'

      es = new EventSource(url)

      es.onmessage = (e) => {
        try {
          const parsed = JSON.parse(e.data) as { type: string; payload: unknown }
          onEventRef.current(parsed)
        } catch {
          // ignore malformed events
        }
      }

      es.onerror = () => {
        es?.close()
        es = null
        if (!stopped) {
          retryTimeout = setTimeout(() => {
            retryDelay = Math.min(retryDelay * 2, 30000)
            connect()
          }, retryDelay)
        }
      }

      es.onopen = () => {
        retryDelay = 1000
      }
    }

    connect()

    return () => {
      stopped = true
      if (retryTimeout) clearTimeout(retryTimeout)
      es?.close()
    }
  }, [])
}
