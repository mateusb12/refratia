import { useEffect, useState, type ReactNode } from 'react'
import clsx from 'clsx'
import {
  Activity,
  AlertTriangle,
  ArrowLeft,
  Calculator,
  Check,
  CircleCheck,
  Database,
  FileCheck,
  FileQuestion,
  FileSearch,
  FileText,
  Info,
  LayoutDashboard,
  Map,
  Moon,
  Plus,
  Settings,
  ShieldAlert,
  ShieldCheck,
  Stethoscope,
  Sun,
  UploadCloud,
  UserRound,
  X,
} from 'lucide-react'
import patientData from '../data/paciente_compilado.json'
import RoadmapPage from './components/roadmap/RoadmapPage'

type Theme = 'light' | 'dark'
type CaseKind = 'demo' | 'real'
type NoticeTone = 'information' | 'warning' | 'blocking'
type DataKind = 'Dado bruto' | 'Dado calculado' | 'Dado ausente'
type Confidence = 'Consistente' | 'Suspeita — revisar'
type Eye = 'OD' | 'OS'

const eyeLabel = (eye: Eye) => (eye === 'OS' ? 'OE' : eye)
const API_URL = import.meta.env.VITE_API_URL ?? (
  window.location.hostname === 'localhost' ? 'http://localhost:3000' : 'https://backend-dry-island-4275.fly.dev'
)

interface Metric {
  label: string
  value: string
  detail: string
  tone: 'default' | 'success' | 'warning'
}

interface DocumentReview {
  name: string
  filename: string
  status: 'Processado' | 'Atenção' | 'Não enviado'
  detail: string
}

interface ExtractedDatum {
  name: string
  fullName?: string
  value: string
  unit?: string
  source: string
  kind: DataKind
  confidence: Confidence
  document: string
  screen: string
  field: string
  crossCheck?: string
  formula?: string
}

interface IntakeAnalysis {
  patient?: { full_name?: string; birth_date?: string }
  exams?: Record<string, unknown>
  [key: string]: unknown
}

interface IntakePreview {
  files: Array<{ filename: string; contentType: string; size: number; sha256: string }>
  analysis: IntakeAnalysis
  message: string
}

function isIntakePreview(value: unknown): value is IntakePreview {
  if (!value || typeof value !== 'object') return false
  const result = value as Partial<IntakePreview>
  return Array.isArray(result.files) && typeof result.analysis === 'object' && result.analysis !== null && typeof result.message === 'string'
}

const steps = [
  { title: 'Enviar exames', description: 'Carregar o caso' },
  { title: 'Conferir documentos', description: 'Identidade e arquivos' },
  { title: 'Conferir dados', description: 'Valores e origem' },
  { title: 'Ver recomendação', description: 'Critérios utilizados' },
  { title: 'Revisar relatório', description: 'Decisão médica' },
]

const metrics: Metric[] = [
  { label: 'Documentos recebidos', value: '4 de 5', detail: 'Um documento não enviado', tone: 'default' as const },
  { label: 'Dados disponíveis', value: '8 de 9', detail: 'Um parâmetro ausente', tone: 'success' as const },
  { label: 'Pontos de atenção', value: '2', detail: 'Sem bloqueio no caso principal', tone: 'warning' as const },
  { label: 'Revisão médica', value: 'Pendente', detail: 'Conclusão parcial', tone: 'warning' as const },
]

const documents: DocumentReview[] = [
  {
    name: 'Pentacam OD',
    filename: 'pentacam-od.pdf',
    status: 'Processado',
    detail: 'Arquivo demonstrativo legível • 6 páginas',
  },
  {
    name: 'Pentacam OE',
    filename: 'pentacam-os.pdf',
    status: 'Processado',
    detail: 'Arquivo demonstrativo legível • 6 páginas',
  },
  {
    name: 'Biometria',
    filename: 'biometria.pdf',
    status: 'Processado',
    detail: 'Arquivo demonstrativo legível • 2 páginas',
  },
  {
    name: 'Microscopia especular',
    filename: 'microscopia.pdf',
    status: 'Processado',
    detail: 'Arquivo demonstrativo legível.',
  },
  {
    name: 'OCT de retina',
    filename: 'oct-retina.pdf',
    status: 'Não enviado',
    detail: 'Nenhum arquivo associado ao caso demonstrativo.',
  },
]

const extractedData: ExtractedDatum[] = [
  {
    name: 'Paquimetria mínima',
    value: '521',
    unit: 'µm',
    source: 'Pentacam OD',
    kind: 'Dado bruto',
    confidence: 'Consistente',
    document: 'Pentacam',
    screen: 'Corneal Thickness',
    field: 'Pachy Min.',
  },
  {
    name: 'Kmax',
    fullName: 'Ceratometria máxima',
    value: '44,2',
    unit: 'D',
    source: 'Pentacam OD',
    kind: 'Dado bruto',
    confidence: 'Consistente',
    document: 'Pentacam',
    screen: 'Topometric',
    field: 'Kmax',
  },
  {
    name: 'BAD-D',
    fullName: 'Belin/Ambrósio Enhanced Ectasia Display',
    value: '1,18',
    source: 'Pentacam OD',
    kind: 'Dado bruto',
    confidence: 'Consistente',
    document: 'Pentacam',
    screen: 'Belin/Ambrósio',
    field: 'Final D',
  },
  {
    name: 'ARTmax',
    fullName: 'Ambrósio Relational Thickness máximo',
    value: 'Não identificado',
    source: 'Pentacam OD',
    kind: 'Dado ausente',
    confidence: 'Suspeita — revisar',
    document: 'Pentacam',
    screen: 'Belin/Ambrósio',
    field: 'ARTmax',
  },
  {
    name: 'ACD',
    fullName: 'Profundidade da Câmara Anterior',
    value: '3,14',
    unit: 'mm',
    source: 'Pentacam OD',
    kind: 'Dado bruto',
    confidence: 'Consistente',
    document: 'Pentacam',
    screen: 'Cataract Pre-OP',
    field: 'Int.',
    crossCheck: 'ACD Ext. - ACD Int. compatível com a paquimetria central',
  },
  {
    name: 'PTA',
    fullName: 'Percentual de Tecido Alterado',
    value: '34,8',
    unit: '%',
    source: 'Pentacam + planejamento',
    kind: 'Dado calculado',
    confidence: 'Consistente',
    document: 'Cálculo RefratIA',
    screen: 'Planejamento demonstrativo',
    field: 'PTA',
    formula: 'PTA = (flap + ablação) / paquimetria × 100',
  },
  {
    name: 'LER',
    fullName: 'Leito Estromal Residual',
    value: '286',
    unit: 'µm',
    source: 'Pentacam + planejamento',
    kind: 'Dado calculado',
    confidence: 'Consistente',
    document: 'Cálculo RefratIA',
    screen: 'Planejamento demonstrativo',
    field: 'LER',
    formula: 'LER = paquimetria − flap − ablação',
  },
  {
    name: 'K final',
    fullName: 'Ceratometria final estimada',
    value: '42,9',
    unit: 'D',
    source: 'Pentacam + planejamento',
    kind: 'Dado calculado',
    confidence: 'Consistente',
    document: 'Cálculo RefratIA',
    screen: 'Planejamento demonstrativo',
    field: 'K final',
    formula: 'K final = K pré-operatório − alteração ceratométrica estimada',
  },
  {
    name: 'Astigmatismo corneano',
    value: '1,31',
    unit: 'D',
    source: 'Pentacam OD',
    kind: 'Dado bruto',
    confidence: 'Consistente',
    document: 'Pentacam',
    screen: 'Cataract Pre-OP',
    field: 'Astig.',
  },
]

const realPatientName = 'Gerinaldo Alfregildo'
const endothelialCutoff = 2000
const realDocuments: DocumentReview[] = patientData.source_files.map((file) => {
  return {
    name: `${file.exam.charAt(0).toUpperCase()}${file.exam.slice(1)} · ${eyeLabel(file.eye as Eye)}`,
    filename: file.path.split('/').pop() ?? file.path,
    status: 'Processado',
    detail: `${file.type === 'application/pdf' ? `${file.pages} páginas` : 'Imagem'} disponível no caso.`,
  }
})

const realExtractedData: ExtractedDatum[] = (['OD', 'OS'] as const).flatMap((eye) => {
  const pentacam = patientData.exams.pentacam.eyes[eye]
  const biometry = patientData.exams.biometry_iol.eyes[eye]
  const microscopy = patientData.exams.specular_microscopy.eyes[eye]
  const label = eyeLabel(eye)
  const source = `Pentacam ${label}`

  return [
    {
      name: `Paquimetria mínima · ${label}`,
      value: pentacam.pachymetry.thinnest_um.toLocaleString('pt-BR'),
      unit: 'µm',
      source,
      kind: 'Dado bruto',
      confidence: 'Consistente',
      document: pentacam.source_file.split('/').pop() ?? source,
      screen: 'Pachymetry',
      field: 'Thinnest',
    },
    {
      name: `Kmax · ${label}`,
      fullName: 'Ceratometria máxima',
      value: pentacam.anterior_cornea.kmax_d.toLocaleString('pt-BR'),
      unit: 'D',
      source,
      kind: 'Dado bruto',
      confidence: 'Consistente',
      document: pentacam.source_file.split('/').pop() ?? source,
      screen: 'Topometric',
      field: 'Kmax',
    },
    {
      name: `BAD-D · ${label}`,
      fullName: 'Belin/Ambrósio Enhanced Ectasia Display',
      value: pentacam.belin_ambrosio.d.toLocaleString('pt-BR'),
      source,
      kind: 'Dado bruto',
      confidence: 'Consistente',
      document: pentacam.source_file.split('/').pop() ?? source,
      screen: 'Belin/Ambrósio',
      field: 'Final D',
    },
    {
      name: `ARTmax · ${label}`,
      fullName: 'Ambrósio Relational Thickness máximo',
      value: pentacam.belin_ambrosio.art_max.toLocaleString('pt-BR'),
      source,
      kind: 'Dado bruto',
      confidence: 'Consistente',
      document: pentacam.source_file.split('/').pop() ?? source,
      screen: 'Belin/Ambrósio',
      field: 'ARTmax',
    },
    {
      name: `Celularidade endotelial · ${label}`,
      fullName: 'Densidade celular endotelial',
      value: microscopy.cell_density_cells_per_mm2.toLocaleString('pt-BR'),
      unit: 'células/mm²',
      source: `Microscopia especular ${label}`,
      kind: 'Dado bruto',
      confidence: 'Consistente',
      document: patientData.exams.specular_microscopy.source.file.split('/').pop() ?? source,
      screen: 'NIDEK',
      field: 'Cell Density (CD)',
    },
    {
      name: `Comprimento axial · ${eye}`,
      value: biometry.axial_length_mm.toLocaleString('pt-BR'),
      unit: 'mm',
      source: `Biometria ${label}`,
      kind: 'Dado bruto',
      confidence: 'Consistente',
      document: 'BIO SRK-T AO.pdf',
      screen: 'EyeSuite IOL',
      field: 'Axial length',
    },
  ] satisfies ExtractedDatum[]
})

const realMetrics: Metric[] = [
  {
    label: 'Arquivos recebidos',
    value: String(patientData.source_files.length),
    detail: 'Todos disponíveis para revisão',
    tone: 'success',
  },
  {
    label: 'Dados em destaque',
    value: String(realExtractedData.length),
    detail: 'Parâmetros reais de OD e OE',
    tone: 'success',
  },
  {
    label: 'Qualidade Pentacam',
    value: `${patientData.exams.pentacam.eyes.OD.quality} / ${patientData.exams.pentacam.eyes.OS.quality}`,
    detail: 'OD / OE',
    tone: 'success',
  },
  {
    label: 'Consolidação',
    value: 'Compatível',
    detail: 'Datas normalizadas',
    tone: 'success',
  },
]

const recommendationReasons = [
  'Faixa refracional compatível',
  'LER dentro do limite',
  'PTA dentro do limite',
  'K final dentro da faixa',
  'Astigmatismo corneano acima de 1,00 D',
  'ARTmax não identificado',
]

const recentCases = [
  { initials: 'RA', patient: realPatientName, report: 'Gerado', review: 'Pendente', tone: 'warning' as const, real: true },
  { initials: 'MS', patient: 'Maria S.', report: 'Parcial', review: 'Pendente', tone: 'warning' as const, real: false },
  { initials: 'JL', patient: 'João L.', report: 'Gerado', review: 'Revisado', tone: 'success' as const, real: false },
  { initials: 'AR', patient: 'Ana R.', report: 'Bloqueado', review: 'Identidade', tone: 'blocking' as const, real: false },
]

function getInitialTheme(): Theme {
  const storedTheme = localStorage.getItem('refratia-theme')
  if (storedTheme === 'light' || storedTheme === 'dark') return storedTheme
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
}

function Eyebrow({ children }: { children: ReactNode }) {
  return <span className="text-primary text-xs font-bold tracking-[0.13em]">{children}</span>
}

function PrimaryButton({ children, onClick, disabled }: { children: ReactNode; onClick?: () => void; disabled?: boolean }) {
  return (
    <button
      className="inline-flex items-center justify-center gap-2 rounded-[9px] border-0 bg-primary px-4 py-[11px] text-sm font-semibold text-white transition-[background,transform] duration-150 hover:enabled:-translate-y-px hover:enabled:bg-primary-hover disabled:cursor-not-allowed disabled:opacity-45"
      disabled={disabled}
      onClick={onClick}
      type="button"
    >
      {children}
    </button>
  )
}

function StatusBadge({ children, tone }: { children: ReactNode; tone: 'success' | 'warning' | 'blocking' | 'neutral' }) {
  return (
    <span className={clsx(
      'inline-flex w-fit items-center gap-1.5 rounded-full px-2.5 py-1.5 text-xs font-bold',
      tone === 'success' && 'bg-success-soft text-success',
      tone === 'warning' && 'bg-warning-soft text-warning',
      tone === 'blocking' && 'bg-danger-soft text-danger',
      tone === 'neutral' && 'bg-surface-muted text-text-secondary',
    )}>
      {children}
    </span>
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

function ProcessSteps({ current, reviewed }: { current: number; reviewed: boolean }) {
  return (
    <ol className="mt-6 grid list-none grid-cols-5 gap-2 p-0 max-[1100px]:grid-cols-2 max-[580px]:grid-cols-1" aria-label="Etapas da análise">
      {steps.map((step, index) => {
        const number = index + 1
        const completed = current > number || (number === steps.length && reviewed)
        const active = current === number && !completed
        return (
          <li
            key={step.title}
            aria-current={active ? 'step' : undefined}
            className={clsx(
              'flex min-w-0 items-center gap-2.5 rounded-[10px] border p-[11px]',
              active && 'border-primary-border bg-primary-soft',
              completed && 'border-border bg-surface-muted',
              !active && !completed && 'border-border bg-surface opacity-60',
            )}
          >
            <span className={clsx(
              'grid h-7 w-7 flex-none place-items-center rounded-full border text-xs font-bold',
              active && 'border-primary bg-primary text-white',
              completed && 'border-success bg-success text-white',
              !active && !completed && 'border-border-strong bg-surface text-text-secondary',
            )}>
              {completed ? <Check size={14} strokeWidth={3} /> : number}
            </span>
            <span className="min-w-0">
              <strong className="block truncate text-xs">{step.title}</strong>
              <small className="mt-0.5 block truncate text-xs text-text-muted">{step.description}</small>
            </span>
          </li>
        )
      })}
    </ol>
  )
}

function ClinicalNotice({ children, tone }: { children: ReactNode; tone: NoticeTone }) {
  const Icon = tone === 'information' ? Info : tone === 'warning' ? AlertTriangle : ShieldAlert
  return (
    <div className={clsx(
      'flex items-start gap-3 rounded-xl border p-4 text-sm leading-relaxed',
      tone === 'information' && 'border-primary-border bg-primary-soft text-text-secondary',
      tone === 'warning' && 'border-warning/30 bg-warning-soft text-text-secondary',
      tone === 'blocking' && 'border-danger/30 bg-danger-soft text-text-secondary',
    )}>
      <Icon className={clsx(
        'mt-0.5 flex-none',
        tone === 'information' ? 'text-primary' : tone === 'warning' ? 'text-warning' : 'text-danger',
      )} size={19} />
      <span>{children}</span>
    </div>
  )
}

function InformationNotice({ children }: { children: ReactNode }) {
  return <ClinicalNotice tone="information">{children}</ClinicalNotice>
}

function WarningNotice({ children }: { children: ReactNode }) {
  return <ClinicalNotice tone="warning">{children}</ClinicalNotice>
}

function BlockingNotice({ children }: { children: ReactNode }) {
  return <ClinicalNotice tone="blocking">{children}</ClinicalNotice>
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
                {expanded === document.name ? 'Ocultar detalhes' : 'Ver detalhes'}
              </button>
              {expanded === document.name && (
                <p className="mb-0 mt-2 border-t border-border pt-3 text-xs leading-relaxed text-text-secondary">
                  {document.detail}
                </p>
              )}
            </article>
          )
        })}
      </div>

      <div className="mt-4">
        <InformationNotice>
          OCT de retina não enviado. A ausência deste dado não altera a recomendação atual.
        </InformationNotice>
      </div>
    </section>
  )
}

function DataKindBadge({ kind }: { kind: DataKind }) {
  return (
    <StatusBadge tone={kind === 'Dado calculado' ? 'success' : kind === 'Dado ausente' ? 'warning' : 'neutral'}>
      {kind === 'Dado calculado' ? <Calculator size={12} /> : kind === 'Dado ausente' ? <FileQuestion size={12} /> : <Database size={12} />}
      {kind}
    </StatusBadge>
  )
}

function ExtractedDataReview({
  items,
  isReal,
  onTrace,
}: {
  items: ExtractedDatum[]
  isReal: boolean
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
        {items.map((data) => (
          <article
            key={data.name}
            className={clsx(
              'flex min-w-0 flex-col rounded-xl border bg-surface-muted p-4',
              data.kind === 'Dado ausente' ? 'border-warning/40' : 'border-border',
            )}
          >
            <div className="flex items-start justify-between gap-3">
              <div className="min-w-0">
                <h3 className="m-0 text-base font-bold">{data.name}</h3>
                {data.fullName && <p className="mb-0 mt-1 text-xs leading-snug text-text-muted">{data.fullName}</p>}
              </div>
              <strong className={clsx(
                'whitespace-nowrap font-display text-lg',
                data.kind === 'Dado ausente' && 'text-warning text-xs',
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
              {data.kind === 'Dado calculado' ? 'Ver cálculo' : 'Ver origem'}
            </button>
          </article>
        ))}
      </div>

      <div className="mt-4">
        {isReal ? (
          <InformationNotice>
            Este primeiro recorte mostra paquimetria, Kmax, BAD-D, ARTmax e comprimento axial dos dois olhos.
          </InformationNotice>
        ) : (
          <WarningNotice>
            ARTmax não identificado. Parte da avaliação de ectasia não pôde ser concluída.
          </WarningNotice>
        )}
      </div>
    </section>
  )
}

function FlowConnector({
  variant,
  className,
}: {
  variant: 'straight' | 'to-active' | 'to-center'
  className?: string
}) {
  const paths = {
    straight: 'M50 0 V48',
    'to-active': 'M50 0 V16 C50 21 52 24 56 24 H77 C81 24 83 27 83 32 V48',
    'to-center': 'M83 0 V16 C83 21 81 24 77 24 H56 C52 24 50 27 50 32 V48',
  }
  const arrowheads = {
    straight: 'M49.25 39 L50 48 L50.75 39 L50 41 Z',
    'to-active': 'M82.25 39 L83 48 L83.75 39 L83 41 Z',
    'to-center': 'M49.25 39 L50 48 L50.75 39 L50 41 Z',
  }

  const renderSvg = (path: string, arrowhead: string, svgClassName?: string) => (
    <svg className={clsx('h-full w-full', svgClassName)} preserveAspectRatio="none" viewBox="0 0 100 48">
      <path d={path} fill="none" shapeRendering="geometricPrecision" stroke="currentColor" strokeLinecap="round" strokeWidth="1.7" vectorEffect="non-scaling-stroke" />
      <path d={arrowhead} fill="currentColor" />
    </svg>
  )

  return (
    <div aria-hidden="true" className={clsx('h-14 w-full text-primary', className)}>
      {variant === 'to-active' ? (
        <>
          {renderSvg(paths[variant], arrowheads[variant], 'max-[820px]:hidden')}
          {renderSvg(paths.straight, arrowheads.straight, 'hidden max-[820px]:block')}
        </>
      ) : renderSvg(paths[variant], arrowheads[variant])}
    </div>
  )
}

function RealCaseSummary() {
  const eyes = (['OD', 'OS'] as const).map((eye) => {
    const pentacam = patientData.exams.pentacam.eyes[eye]
    const biometry = patientData.exams.biometry_iol.eyes[eye]
    const endothelium = patientData.exams.specular_microscopy.eyes[eye]
    const astigmatism = Math.abs(biometry.keratometry.astigmatism_d)
    const coma = Number.parseFloat(pentacam.corneal_rings.zernike['5mm'].z31_coma)

    return {
      eye,
      astigmatism,
      coma,
      endothelialDensity: endothelium.cell_density_cells_per_mm2,
      badD: pentacam.belin_ambrosio.d,
      artMax: pentacam.belin_ambrosio.art_max,
      tkc: pentacam.topometric_indices_8mm.tkc,
      z40: pentacam.cataract_preop.total_corneal_z40_6mm_um,
      recommendation: astigmatism >= 0.75
        ? 'LIO multifocal tórica ou EDOF tórica'
        : 'LIO multifocal ou EDOF não tórica',
    }
  })

  return (
    <section className="mt-5 rounded-2xl border border-border bg-surface p-6 shadow-sm max-[580px]:p-4">
      <Eyebrow>ETAPA 4 · COMO O PROTOCOLO CHEGOU À INDICAÇÃO</Eyebrow>
      <div className="mt-1 flex items-start justify-between gap-4 max-[820px]:flex-col">
        <div>
          <h2 className="m-0 font-display text-2xl tracking-[-0.03em]">Acompanhe a decisão, passo a passo</h2>
          <p className="mb-0 mt-2 max-w-[760px] text-sm leading-relaxed text-text-secondary">
            Cada etapa mostra a pergunta do protocolo, o dado usado e o efeito na recomendação.
          </p>
        </div>
      </div>

      <div className="mt-5 rounded-xl border border-primary-border bg-primary-soft p-5">
        <span className="text-xs font-bold tracking-[0.12em] text-primary">RESULTADO EM UMA FRASE</span>
        <p className="mb-0 mt-2 font-display text-lg leading-snug">
          A idade direciona o caso para implante de lente; a biometria indica lente <strong>{eyes[0].astigmatism >= 0.75 ? 'tórica' : 'não tórica'} no OD</strong> e <strong>{eyes[1].astigmatism >= 0.75 ? 'tórica' : 'não tórica'} no OE</strong>.
        </p>
        <p className="mb-0 mt-2 text-xs leading-relaxed text-text-secondary">
          Multifocal ou EDOF permanece uma escolha do cirurgião, conforme o perfil visual do paciente.
        </p>
      </div>

      <div aria-label="Fluxograma da decisão do protocolo" className="mt-6">
        <div className="mx-auto max-w-[760px] rounded-xl border border-primary bg-primary-soft p-4">
          <span className="text-xs font-bold tracking-[0.12em] text-primary">INÍCIO</span>
          <strong className="mt-1 block text-base">Idade: 59 anos → o protocolo escolhe o Fluxo C</strong>
          <span className="mt-1 block text-xs text-text-secondary">Regra: acima de 55 anos, LASIK e PRK não entram na cascata.</span>
        </div>

        <FlowConnector variant="to-active" />

        <div className="grid grid-cols-3 gap-3 max-[820px]:grid-cols-1">
          <article className="min-h-[134px] rounded-xl border border-border bg-surface-muted p-4 text-text-muted" title="Aplicável apenas a pacientes com menos de 40 anos.">
            <span className="text-xs font-bold tracking-[0.12em]">FLUXO A</span>
            <h3 className="mb-0 mt-1 text-base font-bold">Córnea ou lente fácica</h3>
            <p className="mb-0 mt-2 text-xs">Idade &lt; 40 anos · não escolhido: 59 anos</p>
          </article>

          <article className="min-h-[134px] rounded-xl border border-border bg-surface-muted p-4 text-text-muted" title="Aplicável a pacientes entre 40 e 55 anos.">
            <span className="text-xs font-bold tracking-[0.12em]">FLUXO B</span>
            <h3 className="mb-0 mt-1 text-base font-bold">Presbiopia</h3>
            <p className="mb-0 mt-2 text-xs">40 a 55 anos · não escolhido: 59 anos</p>
          </article>

          <article className="min-h-[134px] rounded-xl border border-success/40 bg-success-soft p-4 shadow-sm">
            <div className="flex items-start justify-between gap-3">
              <div>
                <span className="text-xs font-bold tracking-[0.12em] text-success">ROTA ATIVA · FLUXO C</span>
                <h3 className="mb-0 mt-1 text-base font-bold">LIO multifocal ou EDOF</h3>
              </div>
              <StatusBadge tone="success"><Check size={12} /> Selecionado</StatusBadge>
            </div>
            <p className="mb-0 mt-2 text-xs leading-relaxed text-text-secondary">A idade define a categoria da indicação. A escolha entre multifocal e EDOF fica com o cirurgião.</p>
          </article>
        </div>

        <FlowConnector variant="to-center" className="max-[820px]:hidden" />
        <FlowConnector variant="straight" className="hidden max-[820px]:block" />

        <article className="mx-auto max-w-[760px] rounded-xl border border-primary-border bg-surface p-4 shadow-sm">
          <div className="flex items-start justify-between gap-3">
            <div>
              <span className="text-xs font-bold tracking-[0.12em] text-primary">DECISÃO AVALIADA</span>
              <h3 className="mb-0 mt-1 text-base font-bold">Há ceratocone confirmado?</h3>
            </div>
            <StatusBadge tone="success"><Check size={12} /> Não</StatusBadge>
          </div>
          <div className="mt-3 grid grid-cols-2 gap-3 max-[580px]:grid-cols-1">
            {eyes.map(({ eye, badD, artMax, tkc }) => (
              <div className="rounded-lg border border-border bg-surface-muted p-3 text-xs" key={eye}>
                <strong>{eyeLabel(eye)}</strong>
                <span className="mt-1 block text-text-secondary">BAD-D {badD.toLocaleString('pt-BR')} · ARTmax {artMax} µm · TKC {tkc ?? 'em branco'}</span>
                <span className="mt-2 block font-semibold text-text-primary">{eye === 'OS' ? 'Índices suspeitos; acompanhar' : 'Sem confirmação'}</span>
              </div>
            ))}
          </div>
          <p className="mb-0 mt-3 text-xs text-text-secondary">Sem TKC positivo, a regra global não exclui a indicação. O alerta do OE segue registrado, mas não muda a rota.</p>
          <div className="mt-3 grid grid-cols-2 gap-3 max-[580px]:grid-cols-1">
            <div className="min-h-[134px] rounded-lg border border-border bg-surface-muted p-3 text-xs text-text-muted" title="Com ceratocone confirmado, o Fluxo C mantém a indicação de LIO e registra a observação.">
              <strong className="block text-text-secondary">Sim</strong>
              <span className="mt-1 block">Registrar ceratocone; LIO permanece indicada no Fluxo C.</span>
            </div>
            <div className="min-h-[134px] rounded-lg border border-success/40 bg-success-soft p-3 text-xs">
              <span className="flex items-center gap-1 font-bold text-success"><Check size={13} /> Não · rota escolhida</span>
              <strong className="mt-1 block">Seguir para a escolha da lente</strong>
            </div>
          </div>
        </article>

        <div className="mx-auto mt-3 max-w-[760px] rounded-xl border border-primary-border bg-surface p-4">
          <span className="text-xs font-bold tracking-[0.12em] text-primary">DECISÃO AVALIADA</span>
          <h3 className="mb-0 mt-1 text-base font-bold">A celularidade endotelial está abaixo do ponto de corte?</h3>
          <p className="mb-0 mt-1 text-xs text-text-muted">Regra: densidade celular &lt; {endothelialCutoff.toLocaleString('pt-BR')} células/mm² → ponto de atenção.</p>
          <div className="mt-3 grid grid-cols-2 gap-3 max-[580px]:grid-cols-1">
            {eyes.flatMap(({ eye, endothelialDensity }) => {
              const actualBelowCutoff = endothelialDensity < endothelialCutoff
              return [true, false].map((belowCutoff) => {
                const selected = belowCutoff === actualBelowCutoff
                return (
                  <div
                    className={clsx(
                      'rounded-lg border p-3',
                      selected && belowCutoff && 'border-warning/50 bg-warning-soft',
                      selected && !belowCutoff && 'border-success/50 bg-success-soft',
                      !selected && 'border-border bg-surface-muted text-text-muted',
                    )}
                    key={`${eye}-${belowCutoff}`}
                  >
                    <div className="flex items-center justify-between gap-2">
                      <strong>{eyeLabel(eye)} · {belowCutoff ? 'Sim' : 'Não'}</strong>
                      <StatusBadge tone={selected ? (belowCutoff ? 'warning' : 'success') : 'neutral'}>
                        {selected ? 'Selecionado' : 'Não escolhido'}
                      </StatusBadge>
                    </div>
                    <strong className="mt-2 block font-display text-xl">{endothelialDensity.toLocaleString('pt-BR')} células/mm²</strong>
                    <span className="mt-1 block text-xs">{belowCutoff ? 'Registrar ponto de atenção' : 'Celularidade adequada'}</span>
                  </div>
                )
              })
            })}
          </div>
        </div>

        <FlowConnector variant="straight" />

        <article className="mx-auto max-w-[760px] rounded-xl border border-primary-border bg-surface p-4 shadow-sm">
          <span className="text-xs font-bold tracking-[0.12em] text-primary">DECISÃO AVALIADA</span>
          <h3 className="mb-0 mt-1 text-base font-bold">O astigmatismo pede lente tórica?</h3>
          <p className="mb-0 mt-1 text-xs text-text-muted">Regra: biometria óptica ≥ 0,75 D → versão tórica.</p>
          <div className="mt-3 grid grid-cols-2 gap-3 max-[580px]:grid-cols-1">
            {eyes.map(({ eye, astigmatism }) => {
              const isToric = astigmatism >= 0.75
              return (
                <div className={clsx('rounded-lg border p-3', isToric ? 'border-success/40 bg-success-soft' : 'border-border bg-surface-muted')} key={eye}>
                  <div className="flex items-center justify-between gap-2">
                    <strong>{eyeLabel(eye)}</strong>
                    <StatusBadge tone={isToric ? 'success' : 'neutral'}>{isToric ? 'Sim' : 'Não'}</StatusBadge>
                  </div>
                  <strong className="mt-2 block font-display text-xl">{astigmatism.toLocaleString('pt-BR')} D</strong>
                  <span className="mt-1 block text-xs text-text-secondary">{isToric ? 'LIO tórica' : 'LIO não tórica'}</span>
                </div>
              )
            })}
          </div>
        </article>

        <FlowConnector variant="straight" />

        <details className="mx-auto max-w-[760px] rounded-xl border border-border bg-surface-muted p-4">
          <summary className="cursor-pointer list-none text-sm font-bold marker:hidden">Checagens que não alteraram a rota</summary>
          <div className="mt-3 grid grid-cols-2 gap-3 max-[580px]:grid-cols-1">
            {eyes.map(({ eye, coma, endothelialDensity, z40 }) => (
              <ul className="m-0 grid list-none gap-2 rounded-lg border border-border bg-surface p-3 text-xs text-text-secondary" key={eye}>
                <li className="font-bold text-text-primary">{eye}</li>
                <li>Coma {coma.toLocaleString('pt-BR', { minimumFractionDigits: 3 })} µm: sem alerta</li>
                <li>Endotélio {endothelialDensity.toLocaleString('pt-BR')} células/mm²: adequado</li>
                <li>Z40 {z40.toLocaleString('pt-BR', { minimumFractionDigits: 3 })} µm: informativo</li>
              </ul>
            ))}
          </div>
          <p className="mb-0 mt-3 text-xs text-text-muted">O OCT de retina não foi enviado, mas é informativo neste fluxo e não altera a indicação.</p>
        </details>
      </div>

      <div className="mt-6 rounded-xl bg-sidebar px-5 py-5 text-sidebar-text">
        <div className="flex items-center gap-2 text-[#79d4b7]">
          <CircleCheck size={18} />
          <span className="text-xs font-bold tracking-[0.12em]">FIM DO FLUXO · RECOMENDAÇÃO PRELIMINAR</span>
        </div>
        <div className="mt-3 grid grid-cols-2 gap-3 max-[720px]:grid-cols-1">
          {eyes.map(({ eye, recommendation }) => (
            <div className="rounded-lg border border-white/10 bg-white/[0.07] p-3" key={eye}>
              <span className="text-xs text-sidebar-muted">{eye}</span>
              <strong className="mt-1 block text-sm">{recommendation}</strong>
            </div>
          ))}
        </div>
        <p className="mb-0 mt-3 text-xs text-sidebar-muted">Esta trilha explica a saída do protocolo. A conduta clínica depende da revisão do Dr. Tiago.</p>
      </div>
    </section>
  )
}

function RecommendationSummary({ reviewed, onReview }: { reviewed: boolean; onReview: () => void }) {
  return (
    <section className="mt-5 rounded-2xl border border-border bg-surface p-6 shadow-sm max-[580px]:p-4">
      <div className="flex items-start justify-between gap-4 max-[580px]:flex-col">
        <div>
          <Eyebrow>ETAPAS 4 E 5 · RECOMENDAÇÃO PRELIMINAR</Eyebrow>
          <h2 className="mb-0 mt-1 font-display text-2xl tracking-[-0.03em]">LASIK topo-guiado</h2>
          <p className="mb-0 mt-2 max-w-[660px] text-sm leading-relaxed text-text-secondary">
            Sugestão demonstrativa para apoiar a revisão. Não representa diagnóstico ou indicação cirúrgica definitiva.
          </p>
        </div>
        <span className="rounded-lg border border-border bg-surface-muted px-3 py-2 text-xs text-text-muted">
          Score de apoio <strong className="ml-1 text-text-secondary">74/100</strong>
        </span>
      </div>

      <div className="mt-5 grid grid-cols-[minmax(0,1.25fr)_minmax(280px,.75fr)] gap-4 max-[820px]:grid-cols-1">
        <article className="rounded-xl border border-border bg-surface-muted p-5">
          <h3 className="m-0 font-display text-base">Por que esta recomendação foi gerada</h3>
          <ul className="mb-0 mt-4 grid list-none gap-3 p-0">
            {recommendationReasons.map((reason, index) => (
              <li key={reason} className="flex items-center gap-2.5 text-sm text-text-secondary">
                {index === recommendationReasons.length - 1
                  ? <AlertTriangle className="flex-none text-warning" size={16} />
                  : <CircleCheck className="flex-none text-success" size={16} />}
                {reason}
              </li>
            ))}
          </ul>
        </article>

        <aside className="rounded-xl border border-primary-border bg-primary-soft p-5">
          <Stethoscope className="text-primary" size={21} />
          <span className="mt-4 block text-xs font-bold tracking-[0.12em] text-primary">NÍVEL DE CONCLUSÃO</span>
          <strong className="mt-1.5 block font-display text-base">Parcial — requer revisão médica</strong>
          <p className="mb-0 mt-2 text-xs leading-relaxed text-text-secondary">
            A recomendação permanece válida para análise, mas a ausência de ARTmax limita a conclusão.
          </p>
          <div className="mt-5">
            <PrimaryButton disabled={reviewed} onClick={onReview}>
              <Check size={16} />
              {reviewed ? 'Revisão registrada' : 'Revisar relatório'}
            </PrimaryButton>
          </div>
        </aside>
      </div>

      {reviewed && (
        <p className="mb-0 mt-4 flex items-center gap-2 text-xs font-semibold text-success">
          <CircleCheck size={16} /> Revisão registrada somente neste protótipo.
        </p>
      )}

      <div className="mt-5 rounded-xl border border-border bg-surface-muted p-4">
        <div className="mb-3 flex items-center justify-between gap-3">
          <div>
            <span className="text-xs font-bold tracking-[0.12em] text-text-muted">PRÉVIA DE OUTRO CASO</span>
            <h3 className="mb-0 mt-1 font-display text-base">Exemplo de bloqueio</h3>
          </div>
          <StatusBadge tone="blocking">Caso não liberado</StatusBadge>
        </div>
        <BlockingNotice>
          Identidade do paciente não confirmada. Confirme os documentos antes de gerar o laudo definitivo.
        </BlockingNotice>
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

        {data.formula && (
          <div className="mt-5 rounded-xl border border-success/30 bg-success-soft p-4">
            <span className="text-xs font-bold tracking-[0.12em] text-success">FÓRMULA</span>
            <p className="mb-0 mt-2 font-mono text-sm leading-relaxed">{data.formula}</p>
          </div>
        )}
        {data.crossCheck && (
          <div className="mt-5 rounded-xl border border-primary-border bg-primary-soft p-4">
            <span className="text-xs font-bold tracking-[0.12em] text-primary">CHECAGEM CRUZADA</span>
            <p className="mb-0 mt-2 text-sm leading-relaxed text-text-secondary">{data.crossCheck}</p>
          </div>
        )}
      </aside>
    </div>
  )
}

function App() {
  const [theme, setTheme] = useState<Theme>(getInitialTheme)
  const [activeSection, setActiveSection] = useState('Nova análise')
  const [selectedCase, setSelectedCase] = useState<CaseKind | null>(null)
  const [processingStep, setProcessingStep] = useState(0)
  const [expandedDocument, setExpandedDocument] = useState<string | null>(null)
  const [traceData, setTraceData] = useState<ExtractedDatum | null>(null)
  const [isReviewed, setIsReviewed] = useState(false)
  const [intakeFiles, setIntakeFiles] = useState<File[]>([])
  const [intakePreview, setIntakePreview] = useState<IntakePreview | null>(null)
  const [intakeBusy, setIntakeBusy] = useState(false)
  const [intakeMessage, setIntakeMessage] = useState('')

  const reportGenerated = processingStep === steps.length
  const isRealCase = selectedCase === 'real'
  const activeMetrics = isRealCase ? realMetrics : metrics
  const activeDocuments = isRealCase ? realDocuments : documents
  const activeExtractedData = isRealCase ? realExtractedData : extractedData

  useEffect(() => {
    document.documentElement.dataset.theme = theme
    localStorage.setItem('refratia-theme', theme)
  }, [theme])

  useEffect(() => {
    if (!selectedCase || processingStep === steps.length) return
    const timer = window.setTimeout(() => setProcessingStep((step) => step + 1), 220)
    return () => window.clearTimeout(timer)
  }, [processingStep, selectedCase])

  function loadCase(kind: CaseKind) {
    setSelectedCase(kind)
    setProcessingStep(kind === 'real' ? steps.length : 1)
    setIsReviewed(false)
    setExpandedDocument(null)
    setTraceData(null)
  }

  function backToReports() {
    setSelectedCase(null)
    setProcessingStep(0)
    setIsReviewed(false)
    setExpandedDocument(null)
    setTraceData(null)
  }

  async function analyzeIntake() {
    if (!intakeFiles.length) return
    setIntakeBusy(true)
    setIntakeMessage('')
    try {
      const body = new FormData()
      intakeFiles.forEach((file) => body.append('files', file))
      const response = await fetch(`${API_URL}/api/intakes/analyze`, { method: 'POST', body })
      const result: unknown = await response.json()
      const errorMessage = result && typeof result === 'object' && 'error' in result && typeof result.error === 'string' ? result.error : 'Falha na análise'
      if (!response.ok) throw new Error(errorMessage)
      if (!isIntakePreview(result)) throw new Error('A API de análise está desatualizada. Reinicie o backend e tente novamente.')
      setIntakePreview(result)
    } catch (error) {
      setIntakeMessage(error instanceof Error ? error.message : 'Não foi possível analisar os arquivos.')
    } finally {
      setIntakeBusy(false)
    }
  }

  async function confirmIntake() {
    if (!intakePreview) return
    setIntakeBusy(true)
    setIntakeMessage('')
    try {
      const body = new FormData()
      intakeFiles.forEach((file) => body.append('files', file))
      body.append('analysis', JSON.stringify(intakePreview.analysis))
      const response = await fetch(`${API_URL}/api/intakes/confirm`, { method: 'POST', body })
      const result = await response.json()
      if (!response.ok) throw new Error(result.error ?? 'Falha ao salvar o caso')
      setIntakeMessage(`Caso criado (${result.caseId}). Arquivos e paciente_compilado.json salvos no Tigris.`)
      setIntakeFiles([])
      setIntakePreview(null)
    } catch (error) {
      setIntakeMessage(error instanceof Error ? error.message : 'Não foi possível salvar o caso.')
    } finally {
      setIntakeBusy(false)
    }
  }

  const reviewContent = reportGenerated ? (
    <>
      <button
        className="mb-4 inline-flex items-center gap-2 border-0 bg-transparent p-0 text-sm font-semibold text-primary hover:text-primary-hover"
        onClick={backToReports}
        type="button"
      >
        <ArrowLeft size={16} /> Voltar para casos
      </button>
      <div className={clsx(
        'mt-5 flex items-center justify-between gap-6 rounded-2xl border p-6 max-[820px]:flex-col max-[820px]:items-start max-[580px]:p-4',
        isRealCase ? 'border-warning/50 bg-warning-soft' : 'hero-bg border-primary-border',
      )}>
        <div className="flex items-start gap-4">
          <span className={clsx(
            'grid h-10 w-10 flex-none place-items-center rounded-xl bg-surface shadow-sm',
            isRealCase ? 'text-warning' : 'text-primary',
          )}>
            {isRealCase ? <Database size={20} /> : <FileSearch size={20} />}
          </span>
          <div>
            <div className="flex flex-wrap items-center gap-2">
              <h2 className="m-0 font-display text-xl">
                {isRealCase ? realPatientName : 'Revise evidências antes da conclusão clínica'}
              </h2>
              {isRealCase && <StatusBadge tone="warning">CASO REAL</StatusBadge>}
            </div>
            <p className="mb-0 mt-1.5 max-w-[690px] text-sm leading-relaxed text-text-secondary">
              {isRealCase
                ? 'Nascimento: 12/05/1967 · Dados clínicos importados'
                : 'Confira documentos, dados extraídos, cálculos e a justificativa da recomendação preliminar.'}
            </p>
          </div>
        </div>
        {!isRealCase && (
          <div className="flex max-w-[330px] items-center gap-2.5 rounded-[10px] border border-primary-border bg-surface/80 p-3 text-xs leading-relaxed text-text-secondary max-[820px]:max-w-none">
            <ShieldCheck className="flex-none" size={18} />
            Apoio à avaliação clínica. Não constitui diagnóstico, laudo ou indicação cirúrgica.
          </div>
        )}
      </div>

      <div className="mt-5 grid grid-cols-4 gap-4 max-[1100px]:grid-cols-2 max-[580px]:grid-cols-1">
        {activeMetrics.map((metric) => <MetricCard key={metric.label} {...metric} />)}
      </div>

      <DocumentsReview
        items={activeDocuments}
        expanded={expandedDocument}
        onToggle={(name) => setExpandedDocument((current) => current === name ? null : name)}
      />
      <ExtractedDataReview isReal={isRealCase} items={activeExtractedData} onTrace={setTraceData} />
      {isRealCase
        ? <RealCaseSummary />
        : <RecommendationSummary reviewed={isReviewed} onReview={() => setIsReviewed(true)} />}
    </>
  ) : (
    <section className="mt-5 flex items-center justify-between gap-5 rounded-2xl border border-border bg-surface p-6 shadow-sm max-[580px]:flex-col max-[580px]:items-stretch">
      <div>
        <Eyebrow>REVISÃO CLÍNICA</Eyebrow>
        <h2 className="mb-0 mt-1 font-display text-xl">Nenhum caso carregado nesta sessão</h2>
        <p className="mb-0 mt-1.5 text-sm leading-relaxed text-text-secondary">
          Escolha o caso fictício para iniciar uma nova análise.
        </p>
      </div>
    </section>
  )

  const reportsSection = (
    <section className="mt-5 rounded-2xl border border-border bg-surface p-6 shadow-sm">
      <Eyebrow>CASOS DISPONÍVEIS</Eyebrow>
      <h2 className="mb-0 mt-1 font-display text-xl">Escolha um caso para analisar</h2>
      <div className="mt-4">
        {recentCases.map((item) => (
          <button
            key={item.patient}
            disabled={!item.real && item.patient !== 'Maria S.'}
            className={clsx(
              'grid w-full grid-cols-[auto_minmax(0,1fr)_auto_auto] items-center gap-3 rounded-xl border border-border bg-transparent px-3 py-3 text-left max-[580px]:grid-cols-[auto_minmax(0,1fr)]',
              item.real && 'rounded-xl border border-warning/40 bg-warning-soft',
              item.real ? 'hover:border-warning' : item.patient === 'Maria S.' && 'hover:border-primary',
            )}
            onClick={() => {
              if (item.real) loadCase('real')
              if (item.patient === 'Maria S.') {
                setActiveSection('Nova análise')
                loadCase('demo')
              }
            }}
            type="button"
          >
            <span className={clsx(
              'grid h-9 w-9 place-items-center rounded-[10px] bg-surface-muted text-xs font-bold',
              item.real ? 'text-warning' : 'text-primary',
            )}>{item.initials}</span>
            <span className="min-w-0">
              <span className="flex items-center gap-2">
                <strong className="block truncate text-sm">{item.patient}</strong>
                {item.real && <StatusBadge tone="warning">REAL</StatusBadge>}
              </span>
              <small className="text-xs text-text-muted">{item.real ? 'Caso real' : 'Caso demonstrativo'}</small>
            </span>
            <StatusBadge tone={item.report === 'Bloqueado' ? 'blocking' : item.tone}>Relatório: {item.report}</StatusBadge>
            <StatusBadge tone={item.tone}>Revisão: {item.review}</StatusBadge>
          </button>
        ))}
      </div>
    </section>
  )

  return (
    <div className="grid min-h-screen grid-cols-[264px_minmax(0,1fr)] max-[820px]:grid-cols-1">
      <aside className="sidebar-bg sticky top-0 flex h-screen flex-col px-5 pb-[22px] pt-7 text-sidebar-text max-[820px]:static max-[820px]:h-auto max-[820px]:py-4">
        <div className="flex items-center gap-3 px-2 pb-[30px] max-[820px]:pb-0">
          <span className="grid h-[42px] w-[42px] place-items-center rounded-[13px] border border-white/[0.12] bg-white/[0.09] text-[#79d4b7]">
            <Activity size={23} />
          </span>
          <span>
            <strong className="block font-display text-xl tracking-[-0.02em]">RefratIA</strong>
            <small className="text-sm text-sidebar-muted">Revisão clínica preliminar</small>
          </span>
        </div>

        <nav className="flex flex-col gap-[5px] max-[820px]:hidden" aria-label="Navegação principal">
          <span className="mx-3 mb-2 mt-1 text-xs font-bold tracking-[0.14em] text-sidebar-muted">PLATAFORMA</span>
          {[
            { label: 'Visão geral', icon: <LayoutDashboard size={19} /> },
            { label: 'Nova análise', icon: <Plus size={19} /> },
            { label: 'Relatórios', icon: <FileText size={19} /> },
          ].map((item) => (
            <button
              key={item.label}
              className={clsx(
                'flex w-full items-center gap-3 rounded-[10px] border-0 px-[13px] py-[11px] text-left text-sm font-medium',
                activeSection === item.label ? 'bg-[rgb(103_205_171_/_14%)] text-[#9be0c9]' : 'bg-transparent text-sidebar-muted hover:bg-white/[0.05] hover:text-sidebar-text',
              )}
              onClick={() => setActiveSection(item.label)}
              type="button"
            >
              {item.icon}{item.label}
            </button>
          ))}
          <span className="mx-3 mb-2 mt-6 text-xs font-bold tracking-[0.14em] text-sidebar-muted">SISTEMA</span>
          <button className="flex items-center gap-3 rounded-[10px] px-[13px] py-[11px] text-sm text-sidebar-muted hover:bg-white/[0.05]" type="button">
            <Settings size={19} /> Configurações
          </button>
          <button
            aria-current={activeSection === 'Roadmap' ? 'page' : undefined}
            className={clsx(
              'flex w-full items-center gap-3 rounded-[10px] border-0 px-[13px] py-[11px] text-left text-sm font-medium',
              activeSection === 'Roadmap' ? 'bg-[rgb(103_205_171_/_14%)] text-[#9be0c9]' : 'bg-transparent text-sidebar-muted hover:bg-white/[0.05] hover:text-sidebar-text',
            )}
            onClick={() => setActiveSection('Roadmap')}
            type="button"
          >
            <Map size={19} /> Roadmap
          </button>
        </nav>

        <div className="mt-auto max-[820px]:hidden">
          <div className="flex items-center gap-2.5 rounded-xl border border-white/[0.08] bg-white/[0.04] p-3">
            <span className="grid h-[35px] w-[35px] place-items-center rounded-full bg-sidebar-secondary text-[#86d8bd]"><UserRound size={19} /></span>
            <span>
              <strong className="block text-sm">Dr. Tiago</strong>
              <small className="text-xs text-sidebar-muted">Responsável clínico</small>
            </span>
          </div>
        </div>
      </aside>

      <main className="min-w-0">
        <header className="sticky top-0 z-50 flex min-h-[90px] items-center justify-between gap-6 border-b border-border bg-surface px-[clamp(16px,4vw,56px)] py-5 shadow-sm">
          <div>
            <Eyebrow>PROTOCOLO DEMONSTRATIVO</Eyebrow>
            <h1 className="mb-0 mt-1 font-display text-2xl tracking-[-0.035em]">{activeSection}</h1>
          </div>
          <div className="flex items-center gap-3">
            <span className="hidden items-center gap-2 rounded-full border border-border bg-surface px-3 py-2 text-sm text-text-secondary sm:flex">
              <span className={clsx('h-[7px] w-[7px] rounded-full', isRealCase ? 'bg-danger' : 'bg-warning')} />
              {isRealCase ? 'Caso real' : 'Ambiente demonstrativo'}
            </span>
            <button
              aria-label={theme === 'dark' ? 'Ativar tema claro' : 'Ativar tema escuro'}
              className="grid h-[39px] w-[39px] place-items-center rounded-[11px] border border-border bg-surface shadow-sm hover:border-border-strong"
              onClick={() => setTheme((current) => current === 'dark' ? 'light' : 'dark')}
              type="button"
            >
              {theme === 'dark' ? <Sun size={19} /> : <Moon size={19} />}
            </button>
          </div>
        </header>

        <section className="mx-auto w-full max-w-[1320px] overflow-x-hidden px-[clamp(16px,4vw,56px)] pb-14 pt-8">
          {activeSection === 'Roadmap' ? <RoadmapPage /> : <>
          {activeSection === 'Visão geral' && (
            <>
              <section className="flex items-center justify-between gap-5 rounded-2xl border border-border bg-surface p-6 shadow-sm max-[580px]:flex-col max-[580px]:items-stretch">
                <div>
                  <Eyebrow>RESUMO OPERACIONAL</Eyebrow>
                  <h2 className="mb-0 mt-1 font-display text-xl">Fluxo demonstrativo pronto para revisão</h2>
                </div>
                <PrimaryButton onClick={() => setActiveSection('Nova análise')}>Nova análise</PrimaryButton>
              </section>
            </>
          )}

          {activeSection === 'Nova análise' && (
            <>
              {selectedCase && (
                <button
                  className="mb-4 inline-flex items-center gap-2 border-0 bg-transparent p-0 text-sm font-semibold text-primary hover:text-primary-hover"
                  onClick={backToReports}
                  type="button"
                >
                  <ArrowLeft size={16} /> Voltar para casos
                </button>
              )}
              <section className="mt-5 rounded-2xl border border-primary-border bg-primary-soft/30 p-6 shadow-sm max-[580px]:p-4">
                <Eyebrow>NOVO CASO</Eyebrow>
                <h2 className="mb-0 mt-1 font-display text-xl">Envie os arquivos do paciente</h2>
                <p className="mb-0 mt-1.5 text-sm leading-relaxed text-text-secondary">
                  Os arquivos serão analisados primeiro. Nada é salvo no Tigris até você confirmar a prévia.
                </p>
                <label
                  className="mt-5 flex cursor-pointer flex-col items-center justify-center rounded-xl border-2 border-dashed border-primary/40 bg-surface p-8 text-center hover:border-primary max-[580px]:p-6"
                  onDragOver={(event) => event.preventDefault()}
                  onDrop={(event) => {
                    event.preventDefault()
                    setIntakeFiles(Array.from(event.dataTransfer.files))
                    setIntakePreview(null)
                    setIntakeMessage('')
                  }}
                >
                  <UploadCloud className="text-primary" size={30} />
                  <strong className="mt-3 text-sm">Arraste os arquivos aqui ou clique para selecionar</strong>
                  <span className="mt-1 text-xs text-text-muted">PDF, DOC, DOCX, XLS, XLSX, JPG e PNG · até 20 MB por arquivo</span>
                  <input
                    accept=".pdf,.doc,.docx,.xls,.xlsx,.jpg,.jpeg,.png"
                    className="sr-only"
                    multiple
                    onChange={(event) => {
                      setIntakeFiles(Array.from(event.target.files ?? []))
                      setIntakePreview(null)
                      setIntakeMessage('')
                    }}
                    type="file"
                  />
                </label>
                {intakeFiles.length > 0 && (
                  <div className="mt-4 rounded-xl border border-border bg-surface p-4">
                    <div className="flex flex-wrap items-center justify-between gap-3">
                      <strong className="text-sm">{intakeFiles.length} arquivo(s) selecionado(s)</strong>
                      <PrimaryButton disabled={intakeBusy} onClick={analyzeIntake}>
                        {intakeBusy ? 'Analisando…' : 'Analisar arquivos'}
                      </PrimaryButton>
                    </div>
                    <ul className="mb-0 mt-3 space-y-2 text-xs text-text-secondary">
                      {intakeFiles.map((file) => <li key={`${file.name}-${file.size}`}>{file.name} · {(file.size / 1024 / 1024).toFixed(2)} MB</li>)}
                    </ul>
                  </div>
                )}
                {intakePreview && (
                  <div className="mt-4 rounded-xl border border-success/30 bg-success-soft p-4">
                    <div className="flex flex-wrap items-start justify-between gap-3">
                      <div>
                        <strong className="text-sm">{intakePreview.analysis.patient?.full_name || 'Paciente não identificado'}</strong>
                        <p className="mb-0 mt-1 text-xs text-text-secondary">
                          {intakePreview.analysis.patient?.birth_date || 'Nascimento não identificado'} · {Object.keys(intakePreview.analysis.exams ?? {}).length} tipo(s) de exame
                        </p>
                      </div>
                      <StatusBadge tone="success">JSON extraído</StatusBadge>
                    </div>
                    <p className="mb-0 mt-2 text-sm leading-relaxed text-text-secondary">{intakePreview.message}</p>
                    <details className="mt-4 rounded-lg border border-border bg-surface">
                      <summary className="cursor-pointer px-3 py-2 text-xs font-bold text-primary">Ver paciente_compilado.json</summary>
                      <pre className="m-0 max-h-[520px] overflow-auto border-t border-border p-3 text-xs leading-relaxed text-text-secondary">{JSON.stringify(intakePreview.analysis, null, 2)}</pre>
                    </details>
                    <div className="mt-4 flex flex-wrap gap-3">
                      <PrimaryButton disabled={intakeBusy} onClick={confirmIntake}>{intakeBusy ? 'Salvando…' : 'Confirmar criação do caso'}</PrimaryButton>
                      <button className="rounded-[9px] border border-border-strong bg-surface px-4 py-[11px] text-sm font-semibold hover:border-danger hover:text-danger" disabled={intakeBusy} onClick={() => { setIntakeFiles([]); setIntakePreview(null); setIntakeMessage('Arquivos descartados. Nada foi salvo.') }} type="button">Descartar</button>
                    </div>
                  </div>
                )}
                {intakeMessage && <p className="mb-0 mt-3 text-sm font-semibold text-text-secondary">{intakeMessage}</p>}
              </section>
              <section className="mt-5 rounded-2xl border border-border bg-surface p-6 shadow-sm max-[580px]:p-4">
                <div className="flex items-start justify-between gap-4">
                  <div>
                    <Eyebrow>FLUXO DA ANÁLISE</Eyebrow>
                    <h2 className="mb-0 mt-1 font-display text-xl">Escolha um caso para revisar</h2>
                  </div>
                  <StatusBadge tone={reportGenerated ? 'success' : 'neutral'}>
                    {reportGenerated ? 'Pronto para revisar' : 'Aguardando caso'}
                  </StatusBadge>
                </div>
                <ProcessSteps current={processingStep} reviewed={isReviewed} />
                <div className="mt-5 grid gap-4">
                  <button
                    aria-pressed={selectedCase === 'demo'}
                    className={clsx(
                      'grid grid-cols-[auto_minmax(0,1fr)] items-center rounded-xl border bg-surface-muted p-[18px] text-left hover:border-primary hover:bg-primary-soft/80',
                      selectedCase === 'demo' ? 'border-primary ring-2 ring-primary/15' : 'border-border-strong',
                    )}
                    onClick={() => loadCase('demo')}
                    type="button"
                  >
                    <span className="grid h-[45px] w-[45px] place-items-center rounded-xl bg-primary-soft text-primary"><UploadCloud size={25} /></span>
                    <span className="mx-3.5 min-w-0">
                      <strong className="block truncate text-sm">Maria S.</strong>
                      <small className="mt-1 block text-xs leading-relaxed text-text-muted">Caso fictício completo</small>
                    </span>
                  </button>

                </div>
              </section>
            </>
          )}

          {activeSection === 'Relatórios' && (selectedCase === 'real' ? reviewContent : reportsSection)}
          </>}
        </section>
      </main>

      <TraceabilityDrawer data={traceData} onClose={() => setTraceData(null)} />
    </div>
  )
}

export default App
