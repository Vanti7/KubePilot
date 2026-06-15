import { Updates } from './Updates'

// Risks is the Updates triage view preset to critical + high severity findings.
export function Risks() {
  return <Updates initialFilter={{ severities: ['critical', 'high'] }} />
}
