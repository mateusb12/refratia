import clsx from 'clsx'
import { assessExamContract } from '../../contracts/exam-contracts'
import { type IntakeAnalysis, type IntakePreview } from '../../contracts/patient-analysis'
import type { ReactNode } from 'react'
import { formatBirthDate } from '../../utils/date'

function formatContractValue(value: unknown) {
  if (value === null || value === undefined || value === '') return 'não informado'
  if (typeof value === 'object') {
    if ('normalized' in value) return String((value as { normalized?: unknown }).normalized ?? 'não informado')
    return JSON.stringify(value)
  }
  return String(value)
}

function fileIcon(fileName: string) {
  const extension = fileName.split('.').pop()?.toLowerCase()
  return `${import.meta.env.BASE_URL}${extension === 'pdf' ? 'pdf.png' : extension === 'png' ? 'png.png' : 'jpeg.png'}`
}

const examLabels: Record<string, string> = {
  fundus_retinography: 'Retinografia de fundo de olho',
  iol_calculation: 'Cálculo de lente intraocular',
  refractometry: 'Refratometria',
  oct_retina: 'OCT de retina',
  pentacam_corneal_tomography: 'Tomografia corneana Pentacam',
  specular_microscopy: 'Microscopia especular',
}

function StatusBadge({ children, tone }: { children: ReactNode; tone: 'success' | 'warning' | 'blocking' | 'neutral' }) {
  return <span className={clsx('inline-flex w-fit items-center gap-1.5 rounded-full px-2.5 py-1.5 text-xs font-bold', tone === 'success' && 'bg-success-soft text-success', tone === 'warning' && 'bg-warning-soft text-warning', tone === 'blocking' && 'bg-danger-soft text-danger', tone === 'neutral' && 'bg-surface-muted text-text-secondary')}>{children}</span>
}
export function IntakeCompletenessNotice({ analysis }: { analysis: IntakeAnalysis }) {
  const found = Object.keys(analysis.exams ?? {})
  const missing = Object.entries(examLabels).filter(([key]) => !found.includes(key)).map(([key, label]) => ({ key, label }))
  if (!missing.length) return null
  const informativeKeys = new Set(['fundus_retinography', 'oct_retina'])
  const missingRequired = missing.filter(({ key }) => !informativeKeys.has(key))
  const missingInformative = missing.filter(({ key }) => informativeKeys.has(key))
  if (!missingRequired.length) {
    return (
      <div className="mt-4 rounded-xl border border-border bg-surface-muted p-4 text-sm text-text-secondary">
        <strong className="text-primary">Dados informativos pendentes.</strong>
        <p className="mb-0 mt-1 leading-relaxed">
          Não foram enviados: {missingInformative.map(({ label }) => label).join(' · ')}. Conforme o protocolo, esses exames são informativos e sua ausência não altera a conduta cirúrgica.
        </p>
      </div>
    )
  }
  return (
    <div className="mt-4 rounded-xl border border-warning/40 bg-warning-soft p-4 text-sm text-text-secondary">
      <strong className="text-warning">Análise parcial — dados necessários não enviados.</strong>
      <p className="mb-0 mt-1 leading-relaxed">
        Estes exames não foram identificados: {missingRequired.map(({ label }) => label).join(' · ')}. A ausência limita apenas as etapas que dependem deles.
        {missingInformative.length > 0 && ` Também não foram enviados os exames informativos: ${missingInformative.map(({ label }) => label).join(' · ')}; isso não altera a conduta cirúrgica.`}
      </p>
    </div>
  )
}

export function IntakeAnalysisSummary({ analysis }: { analysis: IntakeAnalysis }) {
  const sourceFiles = analysis.source_files ?? []
  const exams = Object.entries(analysis.exams ?? {})
  const notes = analysis.extraction_notes ?? {}

  return (
    <div className="mt-4 space-y-4">
      <div className="grid gap-3 sm:grid-cols-2">
        <div className="rounded-xl border border-border bg-surface p-4">
          <span className="text-xs font-bold uppercase tracking-[0.12em] text-text-muted">Paciente</span>
          <strong className="mt-2 block text-base">{analysis.patient?.full_name || 'Não identificado'}</strong>
          <span className="mt-1 block text-sm text-text-secondary">
            Nascimento: {formatBirthDate(analysis.patient?.birth_date)}
          </span>
        </div>
        <div className="rounded-xl border border-border bg-surface p-4">
          <span className="text-xs font-bold uppercase tracking-[0.12em] text-text-muted">Resumo da extração</span>
          <strong className="mt-2 block text-base">{exams.length} tipo(s) de exame identificado(s)</strong>
          <span className="mt-1 block text-sm text-text-secondary">{sourceFiles.length} arquivo(s) relacionado(s)</span>
        </div>
      </div>

      <div className="rounded-xl border border-border bg-surface p-4">
        <span className="text-xs font-bold uppercase tracking-[0.12em] text-text-muted">Exames encontrados</span>
        <div className="mt-3 grid gap-3 sm:grid-cols-2">
          {exams.length > 0 ? exams.map(([key, exam]) => (
            <div className="rounded-lg border border-border bg-surface-muted p-3" key={key}>
              <strong className="block text-sm">{examLabels[key] ?? key.replaceAll('_', ' ')}</strong>
              {exam?.eyes && (
                <div className="mt-2 flex flex-wrap gap-2">
                  {Object.keys(exam.eyes).map((eye) => <StatusBadge key={eye} tone="neutral">{eye}</StatusBadge>)}
                </div>
              )}
              {exam?.source?.length > 0 && <p className="mb-0 mt-2 truncate text-xs text-text-secondary" title={exam.source.join(', ')}>Fonte: {exam.source.join(', ')}</p>}
            </div>
          )) : <span className="text-sm text-text-secondary">Nenhum exame identificado.</span>}
        </div>
      </div>

      <div className="rounded-xl border border-border bg-surface p-4">
        <span className="text-xs font-bold uppercase tracking-[0.12em] text-text-muted">Arquivos processados</span>
        <div className="mt-3 grid gap-3 sm:grid-cols-2">
          {sourceFiles.map((source, index) => {
            const fileName = String(source.path ?? source.filename ?? `Arquivo ${index + 1}`)
            const pages = Array.isArray(source.pages) ? source.pages.length : null
            return (
              <div className="flex min-w-0 items-center gap-3 rounded-lg border border-border bg-surface-muted p-3" key={`${fileName}-${index}`}>
                <img alt="" className="h-10 w-10 flex-none object-contain" src={fileIcon(fileName)} />
                <div className="min-w-0">
                  <strong className="block truncate text-sm" title={fileName}>{fileName}</strong>
                  <span className="text-xs text-text-secondary">
                    {[source.exam, source.eye, pages ? `${pages} páginas` : null].filter(Boolean).join(' · ') || 'Arquivo processado'}
                  </span>
                </div>
              </div>
            )
          })}
        </div>
      </div>

      {Object.keys(analysis.conventions ?? {}).length > 0 && (
        <div className="rounded-xl border border-border bg-surface p-4">
          <span className="text-xs font-bold uppercase tracking-[0.12em] text-text-muted">Convenções usadas</span>
          <div className="mt-3 grid gap-x-5 gap-y-2 sm:grid-cols-2">
            {Object.entries(analysis.conventions ?? {}).map(([key, value]) => (
              <div className="text-sm" key={key}><strong>{key}:</strong> <span className="text-text-secondary">{String(value)}</span></div>
            ))}
          </div>
        </div>
      )}

      {typeof notes.clinical_use_warning === 'string' && (
        <div className="rounded-xl border border-warning/30 bg-warning-soft p-4 text-sm text-text-secondary">
          <strong className="text-warning">Atenção:</strong> {notes.clinical_use_warning}
        </div>
      )}
      {typeof notes.fallback_warning === 'string' && (
        <div className="rounded-xl border border-warning/30 bg-warning-soft p-4 text-sm text-text-secondary">
          <strong className="text-warning">Análise local preservada:</strong> {notes.fallback_warning}
        </div>
      )}
      {Array.isArray(notes.invalid_exams) && notes.invalid_exams.length > 0 && (
        <div className="rounded-xl border border-danger/30 bg-danger-soft p-4 text-sm text-text-secondary">
          <strong className="text-danger">Exames com estrutura incompleta:</strong>
          <ul className="mb-0 mt-2 list-disc pl-5">
            {notes.invalid_exams.map((item, index) => {
              const invalid = item as Record<string, unknown>
              return <li key={`${String(invalid.exam)}-${index}`}>{String(invalid.exam ?? 'Exame')} — {String(invalid.reason ?? 'motivo não informado')}</li>
            })}
          </ul>
        </div>
      )}
    </div>
  )
}

function extractedPreviewFields(source: Record<string, unknown> | undefined) {
  if (!source) return []
  const ignored = new Set(['index', 'path', 'filename', 'type', 'size_bytes', 'sha256', 'signed_url'])
  const fields: Array<[string, string]> = []

  function visit(value: unknown, path: string) {
    if (fields.length >= 8 || value === null || value === undefined) return
    if (typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean') {
      fields.push([path, String(value)])
      return
    }
    if (Array.isArray(value)) {
      value.slice(0, 3).forEach((item, index) => visit(item, `${path}[${index + 1}]`))
      return
    }
    if (typeof value === 'object') {
      Object.entries(value as Record<string, unknown>).forEach(([key, child]) => {
        if (!ignored.has(key)) visit(child, path ? `${path}.${key}` : key)
      })
    }
  }

  visit(source, '')
  return fields
}

export function IntakeDocumentsDebug({ preview, localPreviews }: { preview: IntakePreview; localPreviews?: Record<string, string> }) {
  const sources = preview.analysis.source_files ?? []

  return (
    <div className="mt-5 rounded-xl border border-border bg-surface p-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <span className="text-xs font-bold uppercase tracking-[0.12em] text-text-muted">Debug por documento</span>
          <h3 className="mb-0 mt-1 font-display text-lg">O que foi lido em cada arquivo</h3>
          <p className="mb-0 mt-1 text-sm text-text-secondary">Compare a prévia original com os campos que entraram na extração.</p>
        </div>
        <StatusBadge tone="success">{preview.files.length} arquivo(s) analisado(s)</StatusBadge>
      </div>

      <div className="mt-4 grid gap-4 lg:grid-cols-2">
        {preview.files.map((file, index) => {
          const source = sources[index] as Record<string, unknown> | undefined
          const previewUrl = file.signed_url ?? localPreviews?.[file.filename]
          const isPdf = file.contentType === 'application/pdf' || file.filename.toLowerCase().endsWith('.pdf')
          const isImage = file.contentType.startsWith('image/')
          const fields = extractedPreviewFields(source)
          const contractAssessment = source ? assessExamContract(preview.analysis, source) : null
          return (
            <article className="overflow-hidden rounded-xl border border-border bg-surface-muted" key={`${file.filename}-${file.sha256}`}>
              <div className="grid min-h-[190px] place-items-center border-b border-border bg-black/[.04] p-3 dark:bg-white/[.03]">
                {previewUrl && isPdf ? (
                  <iframe className="h-[190px] w-full rounded-lg bg-white" src={previewUrl} title={`Prévia de ${file.filename}`} />
                ) : previewUrl && isImage ? (
                  <img alt={`Prévia de ${file.filename}`} className="max-h-[190px] max-w-full rounded-lg object-contain" src={previewUrl} />
                ) : (
                  <div className="grid place-items-center gap-2 text-center">
                    <img alt="" className="h-16 w-16 object-contain" src={fileIcon(file.filename)} />
                    <span className="text-xs text-text-muted">Prévia indisponível para este formato</span>
                  </div>
                )}
              </div>
              <div className="p-4">
                <div className="flex items-start justify-between gap-3">
                  <div className="min-w-0">
                    <strong className="block truncate text-sm" title={file.filename}>{file.filename}</strong>
                    <span className="mt-1 block text-xs text-text-muted">{(file.size / 1024 / 1024).toFixed(2)} MB · {contractAssessment?.contract.label ?? (source?.exam ? String(source.exam) : 'tipo não identificado')}{(contractAssessment?.eye ?? source?.eye) ? ` · ${String(contractAssessment?.eye ?? source?.eye)}` : ''}</span>
                    {file.canonicalFilename && file.canonicalFilename !== file.filename && (
                      <span className="mt-1 block truncate text-[11px] text-primary" title={file.canonicalFilename}>
                        Nome padronizado: {file.canonicalFilename}
                      </span>
                    )}
                  </div>
                  <StatusBadge tone={contractAssessment?.missing.length ? 'warning' : 'success'}>
                    {contractAssessment ? `${contractAssessment.extracted.length}/${contractAssessment.contract.fields.length} campos` : fields.length ? 'Extraído' : 'Sem campos'}
                  </StatusBadge>
                </div>
                {contractAssessment && (
                  <div className="mt-3 overflow-hidden rounded-lg border border-border bg-surface text-xs">
                    <table className="w-full border-collapse text-left">
                      <thead className="bg-surface-muted text-text-secondary">
                        <tr>
                          <th className="w-12 px-3 py-2 font-bold">Status</th>
                          <th className="px-3 py-2 font-bold">Campo do contrato</th>
                          <th className="px-3 py-2 font-bold">Valor</th>
                        </tr>
                      </thead>
                      <tbody>
                        {contractAssessment.contract.fields.map((field) => {
                          const extracted = contractAssessment.extracted.includes(field)
                          const informative = contractAssessment.contract.key === 'fundus_retinography'
                            || contractAssessment.contract.key === 'oct_retina'
                            || ['coma', 'z40'].includes(field.key)
                          const conditional = ['acd', 'cell_density', 'astigmatism', 'astigmatism_axis'].includes(field.key)
                          const missingTone = informative ? 'bg-surface-muted' : conditional ? 'bg-warning-soft/60' : 'bg-danger-soft/60'
                          return (
                            <tr className={extracted ? 'bg-success-soft/60' : missingTone} key={field.key}>
                              <td className="px-3 py-2 text-base" title={extracted ? 'Campo extraído' : informative ? 'Campo informativo ausente' : conditional ? 'Campo condicional ausente' : 'Campo ausente'}>{extracted ? '✅' : informative ? 'ℹ️' : conditional ? '⚠️' : '❌'}</td>
                              <td className={clsx('px-3 py-2 font-semibold', extracted ? 'text-success' : informative ? 'text-text-secondary' : conditional ? 'text-warning' : 'text-danger')}>{field.label}</td>
                              <td className="max-w-[180px] truncate px-3 py-2 text-text-secondary" title={extracted ? formatContractValue(contractAssessment.values[field.key]) : undefined}>{extracted ? formatContractValue(contractAssessment.values[field.key]) : informative ? 'Não avaliado' : conditional ? 'Depende do fluxo' : 'Não encontrado'}</td>
                            </tr>
                          )
                        })}
                      </tbody>
                    </table>
                  </div>
                )}
                <details className="mt-3 rounded-lg border border-border bg-surface">
                  <summary className="cursor-pointer px-3 py-2 text-xs font-bold text-primary">Ver campos identificados ({fields.length})</summary>
                  <div className="border-t border-border px-3 py-2">
                    {fields.length ? fields.map(([key, value]) => (
                      <div className="grid grid-cols-[minmax(0,.9fr)_minmax(0,1.1fr)] gap-3 border-b border-border py-2 text-xs last:border-0" key={`${key}-${value}`}>
                        <span className="break-words font-semibold text-text-secondary">{key}</span>
                        <span className="break-words text-text-primary">{value}</span>
                      </div>
                    )) : <p className="mb-0 py-1 text-xs text-text-secondary">Nenhum campo estruturado foi associado a este arquivo.</p>}
                  </div>
                </details>
                <details className="mt-2 rounded-lg border border-border bg-surface">
                  <summary className="cursor-pointer px-3 py-2 text-xs font-bold text-text-secondary">Ver JSON deste arquivo</summary>
                  <pre className="m-0 max-h-[280px] overflow-auto border-t border-border p-3 text-[11px] leading-relaxed text-text-secondary">{JSON.stringify(source ?? {}, null, 2)}</pre>
                </details>
              </div>
            </article>
          )
        })}
      </div>
    </div>
  )
}

function previewExamLabel(exam: string) {
  const labels: Record<string, string> = {
    pentacam_corneal_tomography: 'Pentacam',
    iol_calculation: 'Cálculo de LIO',
    specular_microscopy: 'Microscopia especular',
  }
  return labels[exam] ?? exam.replaceAll('_', ' ')
}

function previewFieldLabel(field: string) {
  return field.split('.').map((part) => part.replaceAll('_', ' ')).join(' › ')
}

function previewValue(value: unknown) {
  if (value === null || value === undefined || value === '') return '—'
  if (typeof value === 'boolean') return value ? 'Sim' : 'Não'
  if (typeof value === 'number') return value.toLocaleString('pt-BR', { maximumFractionDigits: 4 })
  if (Array.isArray(value)) return String(value.length) + (value.length === 1 ? ' item' : ' itens')
  if (typeof value === 'object') {
    const count = Object.keys(value as Record<string, unknown>).length
    return String(count) + (count === 1 ? ' campo' : ' campos')
  }
  return String(value)
}

function previewKindLabel(kind: string) {
  if (kind === 'added') return 'Novo'
  if (kind === 'changed') return 'Alterado'
  if (kind === 'removed') return 'Removido'
  return 'Inalterado'
}

function previewKindClass(kind: string) {
  if (kind === 'added') return 'border-success/30 bg-success-soft text-success'
  if (kind === 'changed') return 'border-warning/30 bg-warning-soft text-warning'
  if (kind === 'removed') return 'border-danger/30 bg-danger-soft text-danger'
  return 'border-border bg-surface-muted text-text-muted'
}
function IntakeChangeTable({
  rows,
}: {
  rows: NonNullable<IntakePreview['changePreview']>['rows']
}) {
  return (
    <div className="mt-3 overflow-x-auto rounded-xl border border-border">
      <table className="w-full min-w-[820px] border-collapse text-left text-xs">
        <thead className="bg-surface-muted text-text-secondary">
          <tr>
            <th className="px-3 py-2.5 font-bold">Exame</th>
            <th className="px-3 py-2.5 font-bold">Olho</th>
            <th className="px-3 py-2.5 font-bold">Campo</th>
            <th className="px-3 py-2.5 font-bold">Atual</th>
            <th className="px-3 py-2.5 font-bold">Novo</th>
            <th className="px-3 py-2.5 font-bold">Alteração</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((row, index) => (
            <tr
              className="border-t border-border align-top"
              key={`${row.exam}-${row.eye ?? 'AO'}-${row.field}-${index}`}
            >
              <td className="px-3 py-2.5 font-semibold text-text-primary">
                {previewExamLabel(row.exam)}
              </td>
              <td className="px-3 py-2.5 font-mono text-text-secondary">
                {row.eye || '—'}
              </td>
              <td className="max-w-[280px] px-3 py-2.5 text-text-secondary">
                {previewFieldLabel(row.field)}
              </td>
              <td className="max-w-[220px] break-words px-3 py-2.5 font-mono text-text-muted">
                {previewValue(row.before)}
              </td>
              <td className="max-w-[220px] break-words px-3 py-2.5 font-mono text-text-primary">
                {previewValue(row.after)}
              </td>
              <td className="px-3 py-2.5">
                <span className={`inline-flex rounded-full border px-2 py-1 font-bold ${previewKindClass(row.kind)}`}>
                  {previewKindLabel(row.kind)}
                </span>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

export function PatientChangePreview({ preview }: { preview: IntakePreview }) {
  const changes = preview.changePreview
  if (!changes) return null

  const visibleRows = changes.rows.filter((row) => row.kind !== 'unchanged')
  const unchangedRows = changes.rows.filter((row) => row.kind === 'unchanged')
  const noChanges = visibleRows.length === 0

  return (
    <div className="mt-4 rounded-xl border border-border bg-surface p-4">
      <div className="flex flex-wrap items-center gap-2">
        <strong className="mr-1 text-sm text-text-primary">Alterações deste upload</strong>

        <span className="rounded-full border border-success/30 bg-success-soft px-2.5 py-1 text-xs font-bold text-success">
          {changes.added} novos
        </span>

        <span className="rounded-full border border-warning/30 bg-warning-soft px-2.5 py-1 text-xs font-bold text-warning">
          {changes.changed} alterados
        </span>

        <span className="rounded-full border border-danger/30 bg-danger-soft px-2.5 py-1 text-xs font-bold text-danger">
          {changes.removed} removidos
        </span>

        {changes.unchanged > 0 && (
          <span className="rounded-full border border-border bg-surface-muted px-2.5 py-1 text-xs font-bold text-text-muted">
            {changes.unchanged} inalterados
          </span>
        )}
      </div>

      {noChanges ? (
        <p className="mb-0 mt-3 rounded-lg bg-surface-muted p-3 text-sm text-text-secondary">
          Nenhuma alteração clínica detectada. Os arquivos enviados já fazem parte deste paciente.
        </p>
      ) : (
        <IntakeChangeTable rows={visibleRows} />
      )}

      {unchangedRows.length > 0 && (
        <details className="mt-3">
          <summary className="cursor-pointer text-xs font-bold text-primary">
            Mostrar {unchangedRows.length} campo(s) inalterado(s)
          </summary>
          <IntakeChangeTable rows={unchangedRows} />
        </details>
      )}
    </div>
  )
}


