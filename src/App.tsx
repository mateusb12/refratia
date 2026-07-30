import { useEffect, useState, type ReactNode } from 'react'
import clsx from 'clsx'
import {
  Activity,
  AlertTriangle,
  Calculator,
  Check,
  CircleCheck,
  Database,
  FileCheck,
  FileQuestion,
  FileSearch,
  FileText,
  History,
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

interface Metric {
  label: string
  value: string
  detail: string
  tone: 'default' | 'success' | 'warning'
}

interface DocumentReview {
  name: string
  status: 'Processado' | 'Atenção' | 'Não enviado'
  patient: string
  birthDate: string
  confidence: 'Alta' | 'Moderada' | 'Não confirmada'
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
    status: 'Processado',
    patient: 'Maria S.',
    birthDate: '17/08/1987',
    confidence: 'Alta',
    detail: 'Arquivo demonstrativo legível • 6 páginas',
  },
  {
    name: 'Pentacam OE',
    status: 'Atenção',
    patient: 'Maria S',
    birthDate: '17/08/1987',
    confidence: 'Moderada',
    detail: 'Sobrenome abreviado; compatível com os demais documentos.',
  },
  {
    name: 'Biometria',
    status: 'Processado',
    patient: 'Maria S.',
    birthDate: '17/08/1987',
    confidence: 'Alta',
    detail: 'Arquivo demonstrativo legível • 2 páginas',
  },
  {
    name: 'Microscopia especular',
    status: 'Processado',
    patient: 'Maria S.',
    birthDate: 'Não disponível',
    confidence: 'Alta',
    detail: 'Identidade confirmada pelo nome e identificador do caso.',
  },
  {
    name: 'OCT de retina',
    status: 'Não enviado',
    patient: 'Não lido',
    birthDate: 'Não disponível',
    confidence: 'Não confirmada',
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
const realPatientBirthDate = new Date(`${patientData.patient.birth_date}T00:00:00`).toLocaleDateString('pt-BR')
const realDocuments: DocumentReview[] = patientData.source_files.map((file) => ({
  name: `${file.exam.charAt(0).toUpperCase()}${file.exam.slice(1)} · ${file.eye}`,
  status: 'Processado',
  patient: realPatientName,
  birthDate: realPatientBirthDate,
  confidence: 'Alta',
  detail: `${file.type === 'application/pdf' ? `${file.pages} páginas` : 'Imagem'} · ${file.path.split('/').pop()}`,
}))

const realExtractedData: ExtractedDatum[] = (['OD', 'OS'] as const).flatMap((eye) => {
  const pentacam = patientData.exams.pentacam.eyes[eye]
  const biometry = patientData.exams.biometry_iol.eyes[eye]
  const source = `Pentacam ${eye}`

  return [
    {
      name: `Paquimetria mínima · ${eye}`,
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
      name: `Kmax · ${eye}`,
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
      name: `BAD-D · ${eye}`,
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
      name: `ARTmax · ${eye}`,
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
      name: `Comprimento axial · ${eye}`,
      value: biometry.axial_length_mm.toLocaleString('pt-BR'),
      unit: 'mm',
      source: `Biometria ${eye}`,
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
    detail: 'Parâmetros reais de OD e OS',
    tone: 'success',
  },
  {
    label: 'Qualidade Pentacam',
    value: `${patientData.exams.pentacam.eyes.OD.quality} / ${patientData.exams.pentacam.eyes.OS.quality}`,
    detail: 'OD / OS',
    tone: 'success',
  },
  {
    label: 'Revisão médica',
    value: 'Pendente',
    detail: 'Sem recomendação automatizada',
    tone: 'warning',
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
  { initials: 'RA', patient: realPatientName, report: 'Dados importados', review: 'Pendente', tone: 'warning' as const, real: true },
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
  isReal,
  expanded,
  onToggle,
}: {
  items: DocumentReview[]
  isReal: boolean
  expanded: string | null
  onToggle: (name: string) => void
}) {
  const processed = items.filter((document) => document.status === 'Processado').length

  return (
    <section className="mt-5 rounded-2xl border border-border bg-surface p-6 shadow-sm max-[580px]:p-4">
      <Eyebrow>ETAPA 2 · CONFERÊNCIA</Eyebrow>
      <div className="mt-1 flex items-end justify-between gap-4 max-[580px]:items-start">
        <div>
          <h2 className="m-0 font-display text-xl tracking-[-0.025em]">Documentos recebidos</h2>
          <p className="mb-0 mt-1.5 text-sm leading-relaxed text-text-secondary">
            Confira se os arquivos pertencem ao mesmo paciente antes de revisar os parâmetros.
          </p>
        </div>
        <StatusBadge tone={processed === items.length ? 'success' : 'warning'}>{processed} de {items.length} recebidos</StatusBadge>
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
              <dl className="mt-3 grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-2 text-xs">
                <dt className="text-text-muted">Paciente</dt>
                <dd className="m-0 text-right font-semibold">{document.patient}</dd>
                <dt className="text-text-muted">Nascimento</dt>
                <dd className="m-0 text-right font-semibold">{document.birthDate}</dd>
                <dt className="text-text-muted">Identidade</dt>
                <dd className={clsx(
                  'm-0 text-right font-semibold',
                  document.confidence === 'Alta' ? 'text-success' : document.confidence === 'Moderada' ? 'text-warning' : 'text-text-muted',
                )}>
                  {document.confidence}
                </dd>
              </dl>
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
        {isReal ? (
          <InformationNotice>
            Arquivos e identidade disponíveis para conferência.
          </InformationNotice>
        ) : (
          <InformationNotice>
            OCT de retina não enviado. A ausência deste dado não altera a recomendação atual.
          </InformationNotice>
        )}
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

function RealCaseSummary() {
  return (
    <section className="mt-5 rounded-2xl border border-warning/40 bg-surface p-6 shadow-sm max-[580px]:p-4">
      <Eyebrow>PRÓXIMO PASSO CLÍNICO</Eyebrow>
      <h2 className="mb-0 mt-1 font-display text-xl">Dados reais importados; recomendação ainda não calculada</h2>
      <p className="mb-0 mt-2 max-w-[760px] text-sm leading-relaxed text-text-secondary">
        O caso já apresenta identidade, documentos e parâmetros dos dois olhos. As regras de recomendação continuam exclusivas do caso fictício até serem validadas para dados reais.
      </p>
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
    setProcessingStep(1)
    setIsReviewed(false)
    setExpandedDocument(null)
    setTraceData(null)
  }

  const reviewContent = reportGenerated ? (
    <>
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
                ? `Nascimento: ${realPatientBirthDate} · Dados clínicos importados`
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
        isReal={isRealCase}
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
          Escolha o caso fictício ou o caso real.
        </p>
      </div>
      <PrimaryButton onClick={() => loadCase('real')}>Carregar caso real</PrimaryButton>
    </section>
  )

  const historySection = (
    <section className="mt-5 rounded-2xl border border-border bg-surface p-6 shadow-sm">
      <Eyebrow>ATIVIDADE RECENTE</Eyebrow>
      <h2 className="mb-0 mt-1 font-display text-xl">Histórico recente</h2>
      <div className="mt-4">
        {recentCases.map((item) => (
          <div
            key={item.patient}
            className={clsx(
              'grid grid-cols-[auto_minmax(0,1fr)_auto_auto] items-center gap-3 border-t border-border px-3 py-3 first:border-t-0 max-[580px]:grid-cols-[auto_minmax(0,1fr)]',
              item.real && 'rounded-xl border border-warning/40 bg-warning-soft',
            )}
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
          </div>
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
            { label: 'Histórico', icon: <History size={19} /> },
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
              {historySection}
            </>
          )}

          {activeSection === 'Nova análise' && (
            <>
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
                <div className="mt-5 grid grid-cols-2 gap-4 max-[820px]:grid-cols-1">
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

                  <button
                    aria-pressed={selectedCase === 'real'}
                    className={clsx(
                      'relative grid grid-cols-[auto_minmax(0,1fr)] items-center overflow-hidden rounded-xl border bg-warning-soft p-[18px] text-left hover:border-warning',
                      selectedCase === 'real' ? 'border-warning ring-2 ring-warning/20' : 'border-warning/40',
                    )}
                    onClick={() => loadCase('real')}
                    type="button"
                  >
                    <span className="grid h-[45px] w-[45px] place-items-center rounded-xl bg-surface text-warning shadow-sm"><Database size={24} /></span>
                    <span className="mx-3.5 min-w-0">
                      <span className="flex items-center gap-2">
                        <strong className="block truncate text-sm">{realPatientName}</strong>
                        <StatusBadge tone="warning">REAL</StatusBadge>
                      </span>
                      <small className="mt-1 block text-xs leading-relaxed text-text-muted">Caso real completo</small>
                    </span>
                  </button>
                </div>
              </section>
              {reportGenerated && reviewContent}
            </>
          )}

          {activeSection === 'Relatórios' && reviewContent}
          {activeSection === 'Histórico' && historySection}
          </>}
        </section>
      </main>

      <TraceabilityDrawer data={traceData} onClose={() => setTraceData(null)} />
    </div>
  )
}

export default App
