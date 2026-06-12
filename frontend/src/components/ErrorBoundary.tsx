import React from 'react'

interface Props {
  children: React.ReactNode
}

interface State {
  error: Error | null
}

/**
 * ErrorBoundary catches render-time exceptions so a single component error shows
 * a readable message instead of a blank (black) screen. Styles are inline so it
 * renders even if the CSS bundle failed to load.
 */
export class ErrorBoundary extends React.Component<Props, State> {
  state: State = { error: null }

  static getDerivedStateFromError(error: Error): State {
    return { error }
  }

  componentDidCatch(error: Error, info: React.ErrorInfo) {
    // Surfaced in the browser console for debugging.
    console.error('ErrorBoundary caught:', error, info)
  }

  handleReload = () => {
    this.setState({ error: null })
    window.location.reload()
  }

  render() {
    const { error } = this.state
    if (!error) return this.props.children

    return (
      <div
        style={{
          minHeight: '100vh',
          background: '#0f1117',
          color: '#e2e8f0',
          padding: '2rem',
          fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace',
          display: 'flex',
          flexDirection: 'column',
          gap: '1rem',
        }}
      >
        <h1 style={{ fontSize: '1.1rem', color: '#f87171', margin: 0 }}>
          KubePilot — something crashed while rendering
        </h1>
        <p style={{ color: '#94a3b8', fontSize: '0.85rem', margin: 0 }}>
          {error.message}
        </p>
        <pre
          style={{
            background: '#1a1d27',
            border: '1px solid #2a2f3e',
            borderRadius: 8,
            padding: '1rem',
            overflow: 'auto',
            fontSize: '0.75rem',
            color: '#cbd5e1',
            maxHeight: '50vh',
          }}
        >
          {error.stack}
        </pre>
        <button
          onClick={this.handleReload}
          style={{
            alignSelf: 'flex-start',
            background: '#2563eb',
            color: 'white',
            border: 'none',
            borderRadius: 6,
            padding: '0.5rem 1rem',
            cursor: 'pointer',
            fontSize: '0.85rem',
          }}
        >
          Reload
        </button>
      </div>
    )
  }
}
