import { useEffect, useState } from 'react'
import clsx from 'clsx'
import { AlertTriangle, ArrowLeft, Check, Database, FileCheck, FileQuestion, FileSearch, Trash2, X } from 'lucide-react'
import { formatBirthDate } from '../../shared/date'
import { Eyebrow, InformationNotice, StatusBadge } from '../../shared/ui'
import ProtocolDecisionFlow from './ProtocolDecisionFlow'
import {
  getReportDocuments,
  getReportExtractedData,
  getReportMetrics,
  reportHeaderSummary,
  type DataKind,
  type DocumentReview,
  type ExtractedDatum,
  type Metric,
  type ReportData,
} from './reportModel'

export default function ReportView({
  data,
  bundled,
  deleting,
  onBack,
  onDelete,
}: {
  data: ReportData
  bundled: boolean
  deleting: boolean
  onBack: () => void
  onDelete?: () => void
}) {
  const [expandedDocument, setExpandedDocument] = useState<string | null>(null)
  const [traceData, setTraceData] = useState<ExtractedDatum | null>(null)
  const extractedData = getReportExtractedData(data)
  const metrics = getReportMetrics(data, extractedData)

  return (
    <>
      <button
        className="mb-4 inline-flex items-center gap-2 border-0 bg-transparent p-0 text-sm font-semibold text-primary hover:text-primary-hover"
        onClick={onBack}
        type="button"
      >
        <ArrowLeft size={16} /> Voltar para casos
      </button>
      <div className="relative mt-5 flex items-center justify-between gap-6 rounded-2xl border border-warning/50 bg-warning-soft p-6 max-[820px]:flex-col max-[820px]:items-start max-[580px]:p-4">
        <div className="flex min-w-0 flex-1 items-start gap-4">
          <span className="grid h-10 w-10 flex-none place-items-center rounded-xl bg-surface text-warning shadow-sm">
            <Database size={20} />
          </span>
          <div>
            <div className="flex flex-wrap items-center gap-2">
              <h2 className="m-0 font-display text-2xl tracking-[-0.02em]">{data.patient.full_name}</h2>
              <StatusBadge tone="warning">{bundled ? 'CASO REAL' : 'CASO SALVO'}</StatusBadge>
            </div>
            <p className="mb-0 mt-2 max-w-[760px] text-base leading-relaxed text-text-secondary">
              Nascimento: {formatBirthDate(data.patient.birth_date)}
            </p>
            <ReportHeaderSummary data={data} />
          </div>
        </div>
        {onDelete && (
          <button
            className="absolute bottom-5 right-6 inline-flex items-center gap-2 rounded-[9px] border border-danger/50 bg-surface px-3 py-2 text-sm font-semibold text-danger hover:bg-danger-soft disabled:cursor-not-allowed disabled:opacity-50 max-[580px]:bottom-4 max-[580px]:right-4"
            disabled={deleting}
            onClick={onDelete}
            type="button"
          >
            <Trash2 size={16} /> {deleting ? 'Excluindo…' : 'Excluir caso'}
          </button>
        )}
      </div>

      <div className="mt-5 grid grid-cols-3 gap-4 max-[1100px]:grid-cols-2 max-[580px]:grid-cols-1">
        {metrics.map((metric) => <MetricCard key={metric.label} {...metric} />)}
      </div>

      <DocumentsReview
        items={getReportDocuments(data)}
        expanded={expandedDocument}
        onToggle={(name) => setExpandedDocument((current) => current === name ? null : name)}
      />
      <ExtractedDataReview items={extractedData} onTrace={setTraceData} />
      <ProtocolDecisionFlow data={data} />
      <TraceabilityDrawer data={traceData} onClose={() => setTraceData(null)} />
    </>
  )
}

function ReportHeaderSummary({ data }: { data: ReportData }) {
  const summary = reportHeaderSummary(data)
  const rows = [
    ['Idade', summary.age],
    ['Fluxo', summary.flow],
    ['Datas dos exames', summary.dates],
    ['Refração usada', summary.refraction],
    ['Identidade', summary.identity],
  ]

  return (
    <div className="mt-5 grid w-full max-w-none grid-cols-2 gap-x-14 gap-y-5 text-base leading-relaxed text-text-secondary max-[580px]:grid-cols-1">
      {rows.map(([label, value]) => (
        <div className={clsx('min-w-0', label === 'Identidade' && 'col-span-2 max-[580px]:col-span-1')} key={label}>
          <span className="block text-sm font-bold uppercase tracking-[0.08em] text-text-muted">{label}</span>
          <span className="mt-1 block break-words whitespace-pre-line text-base font-medium text-text-primary" title={value}>{value}</span>
        </div>
      ))}
    </div>
  )
}

function MetricCard({ label, value, detail, tone }: Metric) {
  return (
    <article className="relative overflow-hidden rounded-[14px] border border-border bg-surface p-5 shadow-sm">
      <div className={clsx(
        'absolute right-0 top-0 h-16 w-16 rounded-bl-[64px] opacity-60',
        tone === 'warning' ? 'bg-warning-soft' : tone === 'success' ? 'bg-success-soft' : 'bg-primary-soft',
      )} />
      <span className="relative text-xs font-medium text-text-secondary">{label}</span>
      <strong className="relative mt-4 block font-display text-2xl leading-none tracking-[-0.04em]">{value}</strong>
      <span className="relative mt-1.5 block text-xs text-text-muted">{detail}</span>
    </article>
  )
}

function DocumentsReview({
  items,
  expanded,
  onToggle,
}: {
  items: DocumentReview[]
  expanded: string | null
  onToggle: (name: string) => void
}) {
  const received = items.filter((document) => document.status !== 'Não enviado').length
  const selectedDocument = items.find((document) => document.name === expanded)
  const [imageZoom, setImageZoom] = useState(100)
  const selectedIsPdf = selectedDocument?.filename.toLowerCase().endsWith('.pdf')

  useEffect(() => setImageZoom(100), [selectedDocument?.url])

  return (
    <section className="mt-5 rounded-2xl border border-border bg-surface p-6 shadow-sm max-[580px]:p-4">
      <Eyebrow>ETAPA 2 · CONFERÊNCIA</Eyebrow>
      <div className="mt-1 flex items-end justify-between gap-4 max-[580px]:items-start">
        <div>
          <h2 className="m-0 font-display text-xl tracking-[-0.025em]">Documentos recebidos</h2>
          <p className="mb-0 mt-1.5 text-sm leading-relaxed text-text-secondary">
            Arquivos disponíveis no sistema para a revisão deste caso.
          </p>
        </div>
        <StatusBadge tone={received === items.length ? 'success' : 'warning'}>{received} de {items.length} recebidos</StatusBadge>
      </div>

      <div className="mt-5 grid grid-cols-3 gap-3 max-[1100px]:grid-cols-2 max-[580px]:grid-cols-1">
        {items.map((document) => {
          const tone = document.status === 'Processado' ? 'success' : document.status === 'Atenção' ? 'warning' : 'neutral'
          return (
            <article key={document.name} className="rounded-xl border border-border bg-surface-muted p-4">
              <div className="flex items-start justify-between gap-3">
                <span className="grid h-9 w-9 flex-none place-items-center rounded-lg bg-surface text-primary shadow-sm">
                  {document.status === 'Não enviado' ? <FileQuestion size={18} /> : <FileCheck size={18} />}
                </span>
                <StatusBadge tone={tone}>{document.status}</StatusBadge>
              </div>
              <h3 className="mb-0 mt-3 font-display text-sm">{document.name}</h3>
              <p className="mb-0 mt-3 truncate text-xs text-text-muted" title={document.filename}>
                Arquivo: <span className="font-semibold text-text-secondary">{document.filename}</span>
              </p>
              <button
                aria-expanded={expanded === document.name}
                className="mt-4 text-xs font-bold text-primary hover:underline"
                onClick={() => onToggle(document.name)}
                type="button"
              >
                {expanded === document.name ? 'Fechar visualizador' : 'Visualizar arquivo'}
              </button>
              {expanded === document.name && !document.url && (
                <p className="mb-0 mt-2 border-t border-border pt-3 text-xs leading-relaxed text-text-secondary">
                  {document.detail}
                </p>
              )}
            </article>
          )
        })}
      </div>

      {selectedDocument?.url && (
        <div className="mt-5 overflow-hidden rounded-xl border border-border bg-surface-muted">
          <div className="flex items-center justify-between gap-3 border-b border-border px-4 py-3">
            <strong className="truncate text-sm">{selectedDocument.filename}</strong>
            <div className="flex items-center gap-3">
              {!selectedIsPdf && (
                <label className="flex items-center gap-2 text-xs text-text-secondary">
                  Zoom
                  <input aria-label="Zoom da imagem" max="200" min="50" onChange={(event) => setImageZoom(Number(event.target.value))} type="range" value={imageZoom} />
                  {imageZoom}%
                </label>
              )}
              <button className="text-xs font-bold text-primary hover:underline" onClick={() => onToggle(selectedDocument.name)} type="button">Fechar</button>
            </div>
          </div>
          {selectedIsPdf
            ? <iframe className="h-[680px] w-full bg-white" src={selectedDocument.url} title={selectedDocument.filename} />
            : <div className="max-h-[680px] overflow-auto p-4"><img alt={selectedDocument.name} className="block h-auto max-w-none" src={selectedDocument.url} style={{ width: `${imageZoom}%` }} /></div>}
        </div>
      )}

      {!items.some((document) => /oct|retina/i.test(document.name) && document.status !== 'Não enviado') && <div className="mt-4">
        <InformationNotice>
          OCT de retina não enviado. A ausência deste dado não altera a recomendação atual.
        </InformationNotice>
      </div>}
    </section>
  )
}

function DataKindBadge({ kind }: { kind: DataKind }) {
  return (
    <StatusBadge tone={kind === 'Dado ausente' ? 'warning' : 'neutral'}>
      {kind === 'Dado ausente' ? <FileQuestion size={12} /> : <Database size={12} />}
      {kind}
    </StatusBadge>
  )
}

function ExtractedDataReview({
  items,
  onTrace,
}: {
  items: ExtractedDatum[]
  onTrace: (data: ExtractedDatum) => void
}) {
  const consistent = items.filter((data) => data.confidence === 'Consistente').length

  return (
    <section className="mt-5 rounded-2xl border border-border bg-surface p-6 shadow-sm max-[580px]:p-4">
      <Eyebrow>ETAPA 3 · CONFERÊNCIA</Eyebrow>
      <div className="mt-1 flex items-end justify-between gap-4">
        <div>
          <h2 className="m-0 font-display text-xl tracking-[-0.025em]">Dados extraídos</h2>
          <p className="mb-0 mt-1.5 text-sm leading-relaxed text-text-secondary">
            Dado bruto, cálculo e ausência aparecem separados para orientar a revisão.
          </p>
        </div>
        <StatusBadge tone="success">{consistent} consistentes</StatusBadge>
      </div>

      <div className="mt-5 grid grid-cols-3 gap-3 max-[1100px]:grid-cols-2 max-[580px]:grid-cols-1">
        {items.map((data) => {
          const missingValue = data.kind === 'Dado ausente' || data.value.trim().toLocaleLowerCase() === 'não informado'

          return (
            <article
              key={data.name}
              className={clsx(
                'flex min-w-0 flex-col rounded-xl border bg-surface-muted p-4',
                missingValue ? 'border-warning/60 bg-warning-soft/10' : 'border-border',
              )}
            >
            <div className={clsx(
              'flex items-start justify-between gap-3',
              missingValue && 'flex-col gap-1',
            )}>
              <div className="min-w-0">
                <h3 className="m-0 text-base font-bold">{data.name}</h3>
                {data.fullName && <p className="mb-0 mt-1 text-xs leading-snug text-text-muted">{data.fullName}</p>}
              </div>
              <strong className={clsx(
                'whitespace-nowrap font-display text-lg',
                missingValue && 'self-end rounded-md bg-warning-soft px-2 py-1 text-xs text-warning',
              )}>
                {data.value}{data.unit && <small className="ml-1 text-xs text-text-secondary">{data.unit}</small>}
              </strong>
            </div>

            <div className="mt-4 flex flex-wrap gap-2">
              <DataKindBadge kind={data.kind} />
              <StatusBadge tone={data.confidence === 'Consistente' ? 'success' : 'warning'}>
                {data.confidence === 'Consistente' ? <Check size={12} /> : <AlertTriangle size={12} />}
                {data.confidence}
              </StatusBadge>
            </div>
            <p className="mb-0 mt-3 text-xs text-text-muted">Origem: <span className="font-semibold text-text-secondary">{data.source}</span></p>
            <button
              className="mt-auto self-start pt-4 text-xs font-bold text-primary hover:underline"
              onClick={() => onTrace(data)}
              type="button"
            >
              Ver origem
            </button>
            </article>
          )
        })}
      </div>

      <div className="mt-4">
        <InformationNotice>
          Este primeiro recorte mostra paquimetria, Kmax, BAD-D, ARTmax e comprimento axial dos dois olhos.
        </InformationNotice>
      </div>
    </section>
  )
}

function TraceabilityDrawer({ data, onClose }: { data: ExtractedDatum | null; onClose: () => void }) {
  useEffect(() => {
    if (!data) return
    const closeOnEscape = (event: KeyboardEvent) => event.key === 'Escape' && onClose()
    document.body.style.overflow = 'hidden'
    window.addEventListener('keydown', closeOnEscape)
    return () => {
      document.body.style.overflow = ''
      window.removeEventListener('keydown', closeOnEscape)
    }
  }, [data, onClose])

  if (!data) return null

  return (
    <div className="fixed inset-0 z-[100]">
      <button aria-label="Fechar rastreabilidade" className="absolute inset-0 h-full w-full bg-black/40" onClick={onClose} type="button" />
      <aside
        aria-labelledby="traceability-title"
        aria-modal="true"
        className="absolute inset-y-0 right-0 flex w-full max-w-[480px] flex-col overflow-y-auto border-l border-border bg-surface p-6 shadow-md max-[580px]:p-4"
        role="dialog"
      >
        <div className="flex items-start justify-between gap-4">
          <div>
            <Eyebrow>RASTREABILIDADE</Eyebrow>
            <h2 className="mb-0 mt-1 font-display text-xl" id="traceability-title">{data.name}</h2>
            {data.fullName && <p className="mb-0 mt-1 text-sm text-text-secondary">{data.fullName}</p>}
          </div>
          <button
            autoFocus
            aria-label="Fechar painel"
            className="grid h-9 w-9 flex-none place-items-center rounded-lg border border-border bg-surface-muted hover:border-border-strong"
            onClick={onClose}
            type="button"
          >
            <X size={18} />
          </button>
        </div>

        <div className="mt-6 grid min-h-[190px] place-items-center rounded-xl border border-dashed border-border-strong bg-surface-muted text-center">
          <span>
            <FileSearch className="mx-auto text-text-muted" size={25} />
            <strong className="mt-2 block text-sm">Recorte do exame</strong>
            <small className="mt-1 block text-xs text-text-muted">Placeholder demonstrativo</small>
          </span>
        </div>

        <dl className="mt-6 grid grid-cols-[minmax(110px,.7fr)_minmax(0,1.3fr)] gap-x-4 gap-y-3 text-sm">
          {[
            ['Parâmetro', data.name],
            ['Nome completo', data.fullName ?? data.name],
            ['Valor lido', `${data.value}${data.unit ? ` ${data.unit}` : ''}`],
            ['Documento', data.document],
            ['Tela', data.screen],
            ['Campo', data.field],
            ['Tipo', data.kind],
            ['Confiança', data.confidence],
          ].map(([label, value]) => (
            <div className="contents" key={label}>
              <dt className="border-b border-border pb-3 text-text-muted">{label}</dt>
              <dd className="m-0 border-b border-border pb-3 font-semibold">{value}</dd>
            </div>
          ))}
        </dl>

      </aside>
    </div>
  )
}
