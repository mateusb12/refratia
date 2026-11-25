import { useEffect, useState } from 'react'
import { API_URL, API_WRITE_TOKEN } from '../../shared/api'
import { formatCaseSavedTime, type SavedCase } from '../../shared/cases'
import type { IntakeAnalysis } from '../../shared/contracts/patient-analysis'
import { Eyebrow, StatusBadge } from '../../shared/ui'
import ReportErrorBoundary from './ReportErrorBoundary'
import ReportView from './ReportView'
import bundledReport from '../../../data/paciente_compilado.json'
import { BUNDLED_REPORT_ID, normalizeSavedReport, type ReportData } from './reportModel'

async function loadSavedReport(caseId: string): Promise<ReportData> {
  const response = await fetch(`${API_URL}/api/cases/${encodeURIComponent(caseId)}`)
  const body = await response.text()
  let result: unknown
  try {
    result = JSON.parse(body)
  } catch {
    throw new Error(`Resposta inválida do servidor (${response.status}). Publique o backend atualizado.`)
  }
  if (!response.ok || !result || typeof result !== 'object' || !('analysis' in result)) {
    throw new Error('Não foi possível carregar a análise deste caso.')
  }
  return normalizeSavedReport(result.analysis as IntakeAnalysis)
}

async function deleteSavedCase(caseId: string) {
  const response = await fetch(`${API_URL}/api/cases/${encodeURIComponent(caseId)}`, {
    method: 'DELETE',
    headers: { Authorization: `Bearer ${API_WRITE_TOKEN}` },
  })
  const result = await response.json().catch(() => ({}))
  if (!response.ok) throw new Error(result.error ?? 'Não foi possível excluir o caso.')
}

export default function ReportsPage({
  reportId,
  cases,
  onOpen,
  onBack,
  onDeleted,
}: {
  reportId: string | null
  cases: SavedCase[]
  onOpen: (caseId: string) => void
  onBack: () => void
  onDeleted: (caseId: string) => void
}) {
  const [saved, setSaved] = useState<{ caseId: string; data: ReportData } | null>(null)
  const [error, setError] = useState('')
  const [deleting, setDeleting] = useState(false)

  useEffect(() => {
    setError('')
    setSaved(null)
    if (!reportId || reportId === BUNDLED_REPORT_ID) return
    let current = true
    loadSavedReport(reportId)
      .then((data) => current && setSaved({ caseId: reportId, data }))
      .catch((loadError) => current && setError(loadError instanceof Error ? loadError.message : 'Não foi possível carregar o caso.'))
    return () => {
      current = false
    }
  }, [reportId])
  const loading = Boolean(reportId) && reportId !== BUNDLED_REPORT_ID && !saved && !error

  async function remove(caseId: string, patientName: string) {
    if (!window.confirm(`Excluir permanentemente o caso de ${patientName || 'este paciente'}?`)) return
    setDeleting(true)
    setError('')
    try {
      await deleteSavedCase(caseId)
      onDeleted(caseId)
    } catch (deleteError) {
      setError(deleteError instanceof Error ? deleteError.message : 'Não foi possível excluir o caso.')
    } finally {
      setDeleting(false)
    }
  }

  const errorBanner = error && <p className="mb-0 mt-3 rounded-lg border border-danger bg-danger-soft p-3 text-sm font-semibold text-danger">{error}</p>

  if (reportId === BUNDLED_REPORT_ID) {
    return (
      <ReportErrorBoundary key={reportId} onBack={onBack}>
        <ReportView bundled data={bundledReport} deleting={false} onBack={onBack} />
      </ReportErrorBoundary>
    )
  }
  if (saved && saved.caseId === reportId) {
    return (
      <ReportErrorBoundary key={reportId} onBack={onBack}>
        {errorBanner}
        <ReportView
          bundled={false}
          data={saved.data}
          deleting={deleting}
          onBack={onBack}
          onDelete={() => void remove(saved.caseId, saved.data.patient?.full_name)}
        />
      </ReportErrorBoundary>
    )
  }

  return (
    <section className="mt-5 rounded-2xl border border-border bg-surface p-6 shadow-sm">
      <Eyebrow>CASOS DISPONÍVEIS</Eyebrow>
      <h2 className="mb-0 mt-1 font-display text-xl">Escolha um caso para analisar</h2>
      {loading && <p className="mb-0 mt-3 text-sm text-text-secondary">Carregando análise salva…</p>}
      {errorBanner}
      <div className="mt-4">
        {cases.map((item) => (
          <button
            key={item.caseId}
            className="grid w-full grid-cols-[auto_minmax(0,1fr)_auto_auto] items-center gap-3 rounded-xl border border-success/40 bg-success-soft px-3 py-3 text-left hover:border-primary max-[580px]:grid-cols-[auto_minmax(0,1fr)]"
            onClick={() => onOpen(item.caseId)}
            type="button"
          >
            <span className="grid h-9 w-9 place-items-center rounded-[10px] bg-surface-muted text-xs font-bold text-primary">
              {item.patientName.split(/\s+/).map((name) => name[0]).join('').slice(0, 2).toUpperCase()}
            </span>
            <span className="min-w-0">
              <span className="flex items-center gap-2">
                <strong className="block truncate text-sm">{item.patientName}</strong>
                <StatusBadge tone="success">NOVO</StatusBadge>
              </span>
              <small className="text-xs text-text-muted">Caso salvo · {formatCaseSavedTime(item.caseId) ?? item.caseId}</small>
            </span>
            <StatusBadge tone="success">Relatório: Gerado</StatusBadge>
            <StatusBadge tone="success">Revisão: Pendente</StatusBadge>
          </button>
        ))}
      </div>
    </section>
  )
}
