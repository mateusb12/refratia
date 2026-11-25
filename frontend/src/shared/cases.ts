import { useEffect, useState } from 'react'
import { API_URL } from './api'

export interface SavedCase {
  caseId: string
  patientName: string
  analysisKey?: string
}

// Case ids carry a UTC timestamp; the clinic works in UTC-3.
export function formatCaseSavedTime(caseId: string) {
  const match = caseId.match(/^case-(\d{8})T(\d{2})(\d{2})\d{2}Z-/)
  if (!match) return null
  const localHour = (Number(match[2]) + 21) % 24
  return `${String(localHour).padStart(2, '0')}h${match[3]}`
}

export function useCaseList() {
  const [cases, setCases] = useState<SavedCase[]>([])

  useEffect(() => {
    fetch(`${API_URL}/api/cases`)
      .then(async (response) => response.ok ? response.json() : null)
      .then((result) => {
        if (result && Array.isArray(result.cases)) setCases(result.cases)
      })
      .catch(() => undefined)
  }, [])

  return {
    cases,
    add: (newCase: SavedCase) => setCases((current) => [newCase, ...current.filter((item) => item.caseId !== newCase.caseId)]),
    remove: (caseId: string) => setCases((current) => current.filter((item) => item.caseId !== caseId)),
  }
}
