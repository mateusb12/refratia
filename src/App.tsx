import { useEffect, useState, type ReactNode } from 'react'
import clsx from 'clsx'
import {
  Activity,
  AlertTriangle,
  BarChart3,
  Check,
  CircleCheck,
  FileCheck,
  FileSearch,
  FileText,
  History,
  LayoutDashboard,
  Moon,
  Plus,
  Settings,
  ShieldCheck,
  Stethoscope,
  Sun,
  Target,
  UploadCloud,
  UserRound,
} from 'lucide-react'

type Theme = 'light' | 'dark'
type ReportView = 'Resumo' | 'Parâmetros' | 'Protocolo'

interface MetricCardProps {
  icon: ReactNode
  label: string
  value: string
  detail: string
  tone?: 'default' | 'success' | 'warning'
}

interface StepProps {
  number: string
  title: string
  description: string
  active?: boolean
  completed?: boolean
}

const steps = [
  { title: 'Enviar exames', description: 'PDF demonstrativo' },
  { title: 'Extrair dados', description: 'Nome, idade e parâmetros' },
  { title: 'Aplicar protocolo', description: 'RefratIA v0.1' },
  { title: 'Gerar relatório', description: 'Preliminar automatizado' },
]

const metrics = [
  { label: 'Exames processados', value: '24', detail: 'Pentacam demonstrativo' },
  { label: 'Relatórios gerados', value: '24', detail: 'Geração automatizada', tone: 'success' as const },
  { label: 'Aguardando revisão', value: '3', detail: 'Revisão médica posterior', tone: 'warning' as const },
  { label: 'Com alerta de extração', value: '1', detail: 'Dado ausente não impeditivo', tone: 'warning' as const },
]

const extractedData = [
  ['Paciente', 'Maria da Silva', 'Pentacam'],
  ['Idade', '38 anos', 'Pentacam'],
  ['Kmax', '44,2 D', 'Pentacam'],
  ['Paquimetria mínima', '521 µm', 'Pentacam'],
  ['BAD-D', '1,18', 'Pentacam'],
  ['ARTmax', 'Não identificado', 'Pentacam'],
  ['Astigmatismo corneano', '1,31 D', 'Pentacam'],
]

const protocolRules = [
  'Faixa etária entre 18 e 40 anos',
  'Parâmetros corneanos disponíveis sem alerta impeditivo no protocolo',
  'Astigmatismo corneano exige consideração de abordagem personalizada',
  'ARTmax ausente limita parte da avaliação',
]

const reportHighlights = [
  ['521 µm', 'Paquimetria mínima', 'Dentro da zona demonstrativa'],
  ['44,2 D', 'Kmax', 'Sem alerta impeditivo'],
  ['1,18', 'BAD-D', 'Baixo risco no protocolo'],
  ['1,31 D', 'Astigmatismo', 'Pede avaliação personalizada'],
]

const parameterBars = [
  { label: 'Paquimetria mínima', value: '521 µm', percent: 74, detail: 'Boa espessura no caso demonstrativo', tone: 'success' },
  { label: 'Kmax', value: '44,2 D', percent: 58, detail: 'Curvatura sem gatilho crítico', tone: 'success' },
  { label: 'BAD-D', value: '1,18', percent: 42, detail: 'Compatível com fluxo preliminar', tone: 'success' },
  { label: 'ARTmax', value: 'Não identificado', percent: 18, detail: 'Bloqueia conclusões dependentes', tone: 'warning' },
]

const reportViews: ReportView[] = ['Resumo', 'Parâmetros', 'Protocolo']

const recentCases = [
  { initials: 'MS', patient: 'Maria da Silva', exam: 'Pentacam', report: 'Gerado', review: 'Pendente', statusClass: 'warning' },
  { initials: 'JL', patient: 'João Lima', exam: 'Pentacam', report: 'Gerado', review: 'Revisado', statusClass: 'success' },
  { initials: 'AR', patient: 'Ana Rocha', exam: 'Pentacam', report: 'Parcial', review: 'Atenção', statusClass: 'warning' },
]

function getInitialTheme(): Theme {
  const storedTheme = localStorage.getItem('refratia-theme')
  if (storedTheme === 'light' || storedTheme === 'dark') return storedTheme
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
}

function MetricCard({ icon, label, value, detail, tone = 'default' }: MetricCardProps) {
  return (
    <article className="relative overflow-hidden p-5 rounded-[14px] border border-border bg-surface shadow-sm">
      <div
        className={clsx(
          'absolute top-0 right-0 w-20 h-20 rounded-bl-[80px] opacity-55',
          tone === 'warning' ? 'bg-warning-soft' : tone === 'success' ? 'bg-success-soft' : 'bg-primary-soft',
        )}
      />

      <div className="relative z-10 flex items-center gap-2.5">
        <span className={clsx(
          tone === 'warning' ? 'text-warning' : tone === 'success' ? 'text-success' : 'text-primary',
        )}>
          {icon}
        </span>
        <span className="text-text-secondary text-xs font-medium">{label}</span>
      </div>

      <strong className="relative z-10 block mt-5 font-display text-[30px] leading-none tracking-[-0.045em]">
        {value}
      </strong>
      <span className="relative z-10 block mt-1 text-text-muted text-[11px]">{detail}</span>
    </article>
  )
}

function Step({ number, title, description, active = false, completed = false }: StepProps) {
  return (
    <div className={clsx(
      'flex min-w-0 items-center gap-2.5 p-[11px] border rounded-[10px] opacity-[0.72]',
      active && 'border-primary-border bg-primary-soft opacity-100',
      completed && 'border-border bg-surface-muted opacity-100',
      !active && !completed && 'border-border bg-surface-muted max-[580px]:hidden',
    )}>
      <span className={clsx(
        'grid w-[27px] h-[27px] flex-none place-items-center rounded-full border text-[11px] font-bold',
        active && 'border-primary bg-primary text-white',
        completed && 'border-success bg-success text-white',
        !active && !completed && 'border-border-strong bg-surface text-text-secondary',
      )}>
        {completed ? <Check size={15} strokeWidth={3} /> : number}
      </span>

      <div>
        <strong className="block overflow-hidden text-[11px] text-ellipsis whitespace-nowrap">{title}</strong>
        <p className="overflow-hidden m-0 mt-0.5 text-text-muted text-[9px] text-ellipsis whitespace-nowrap">{description}</p>
      </div>
    </div>
  )
}

function Eyebrow({ children }: { children: ReactNode }) {
  return <span className="text-primary text-[10px] font-bold tracking-[0.13em]">{children}</span>
}

function PrimaryButton({ children, onClick, disabled }: { children: ReactNode; onClick?: () => void; disabled?: boolean }) {
  return (
    <button
      className="inline-flex items-center justify-center gap-2 px-4 py-[11px] rounded-[9px] bg-primary text-white text-[11px] font-semibold border-0 cursor-pointer transition-[background,transform] duration-[160ms] ease-[ease] hover:enabled:bg-primary-hover hover:enabled:-translate-y-px disabled:cursor-not-allowed disabled:opacity-45"
      disabled={disabled}
      onClick={onClick}
      type="button"
    >
      {children}
    </button>
  )
}

function CaseStatus({ children, tone }: { children: ReactNode; tone: 'success' | 'warning' }) {
  return (
    <span className={clsx(
      'px-[9px] py-[6px] rounded-full text-[9px] font-semibold',
      tone === 'success' ? 'bg-success-soft text-success' : 'bg-warning-soft text-warning',
    )}>
      {children}
    </span>
  )
}

function App() {
  const [theme, setTheme] = useState<Theme>(getInitialTheme)
  const [selectedFile, setSelectedFile] = useState<string | null>(null)
  const [activeSection, setActiveSection] = useState('Nova análise')
  const [processingStep, setProcessingStep] = useState(0)
  const [isReviewed, setIsReviewed] = useState(false)
  const [reportView, setReportView] = useState<ReportView>('Resumo')

  const reportGenerated = selectedFile !== null && processingStep === steps.length

  useEffect(() => {
    document.documentElement.dataset.theme = theme
    localStorage.setItem('refratia-theme', theme)
  }, [theme])

  function toggleTheme() {
    setTheme((t) => (t === 'dark' ? 'light' : 'dark'))
  }

  function loadDemoCase() {
    setSelectedFile('pentacam-maria-silva.pdf')
    setIsReviewed(false)
    setProcessingStep(1)
    steps.slice(1).forEach((_, index) => {
      window.setTimeout(() => setProcessingStep(index + 2), 180 * (index + 1))
    })
  }

  const reportSection = reportGenerated ? (
    <section className="border border-border bg-surface rounded-2xl shadow-sm p-6 mt-5">
      <div className="flex items-start justify-between gap-[18px]">
        <div>
          <Eyebrow>RESULTADO DEMONSTRATIVO</Eyebrow>
          <h2 className="mt-[5px] mb-0 font-display text-[17px] tracking-[-0.025em]">Relatório clínico interativo</h2>
        </div>

        <span className="inline-flex items-center gap-[7px] px-[10px] py-[7px] rounded-full bg-warning-soft text-warning text-[10px] font-semibold">
          <AlertTriangle size={16} />
          ARTmax exige atenção
        </span>
      </div>

      {/* Score + highlights */}
      <div className="grid mt-5 gap-3.5 grid-cols-[minmax(280px,0.9fr)_minmax(0,1.1fr)] max-[820px]:grid-cols-1">
        <div className="flex items-center gap-4 p-[18px] border border-border rounded-lg bg-surface-muted max-[580px]:flex-col">
          <span className="score-ring grid w-[92px] h-[92px] flex-none place-items-center rounded-full text-primary font-display text-[26px] font-extrabold max-[580px]:w-[78px] max-[580px]:h-[78px] max-[580px]:text-[22px]">
            74
          </span>
          <div>
            <Eyebrow>SCORE PRELIMINAR</Eyebrow>
            <h3 className="mt-[5px] mb-0 font-display text-[18px] tracking-[-0.025em]">Candidato para revisão médica</h3>
            <p className="mt-1.5 mb-0 text-text-secondary text-[11px] leading-[1.55]">
              Parâmetros principais disponíveis, com uma pendência que limita conclusões dependentes de ARTmax.
            </p>
          </div>
        </div>

        <div className="grid grid-cols-2 gap-2.5 max-[580px]:grid-cols-1">
          {reportHighlights.map(([value, label, detail]) => (
            <div key={label} className="border border-border rounded-lg bg-surface-muted p-3.5">
              <strong className="block font-display text-[22px] tracking-[-0.025em]">{value}</strong>
              <span className="block mt-1 text-text-secondary text-[10px] font-bold">{label}</span>
              <small className="block mt-2 text-text-muted text-[10px] leading-[1.45]">{detail}</small>
            </div>
          ))}
        </div>
      </div>

      {/* Tabs */}
      <div className="flex flex-wrap gap-2 mt-[18px] p-1.5 border border-border rounded-[10px] bg-surface-muted" role="tablist" aria-label="Visões do relatório">
        {reportViews.map((view) => (
          <button
            key={view}
            aria-selected={reportView === view}
            className={clsx(
              'inline-flex items-center justify-center gap-[7px] min-h-[34px] px-3 border-0 rounded-[7px] text-[11px] font-bold cursor-pointer transition-none',
              reportView === view
                ? 'bg-surface text-primary shadow-sm'
                : 'bg-transparent text-text-secondary',
            )}
            onClick={() => setReportView(view)}
            role="tab"
            type="button"
          >
            {view === 'Resumo' && <Target size={15} />}
            {view === 'Parâmetros' && <BarChart3 size={15} />}
            {view === 'Protocolo' && <FileCheck size={15} />}
            {view}
          </button>
        ))}
      </div>

      {/* Resumo */}
      {reportView === 'Resumo' && (
        <div className="grid mt-3.5 grid-cols-2 gap-3 max-[820px]:grid-cols-1">
          <article className="flex items-start gap-3 p-[15px] border border-border rounded-lg bg-surface-muted">
            <CircleCheck size={20} className="text-success flex-none mt-0.5" />
            <div>
              <span className="block mt-1 text-text-secondary text-[10px] font-bold">Achado principal</span>
              <strong className="block mt-1 text-[13px]">Sem alerta impeditivo nos dados identificados</strong>
              <p className="mt-1.5 mb-0 text-text-secondary text-[11px] leading-[1.55]">
                Idade, Kmax, paquimetria e BAD-D mantêm o caso no fluxo preliminar para revisão.
              </p>
            </div>
          </article>

          <article className="flex items-start gap-3 p-[15px] border border-border rounded-lg bg-surface-muted">
            <AlertTriangle size={20} className="text-warning flex-none mt-0.5" />
            <div>
              <span className="block mt-1 text-text-secondary text-[10px] font-bold">Pendência</span>
              <strong className="block mt-1 text-[13px]">ARTmax não foi extraído do PDF</strong>
              <p className="mt-1.5 mb-0 text-text-secondary text-[11px] leading-[1.55]">
                O relatório omite qualquer conclusão que dependa desse parâmetro.
              </p>
            </div>
          </article>

          <article className="col-span-2 p-4 border border-border rounded-lg bg-surface-muted">
            <Eyebrow>CONDUTA DO SISTEMA</Eyebrow>
            <h3 className="mt-[5px] mb-0 font-display text-[18px] tracking-[-0.025em]">Gerar relatório parcial e exigir revisão</h3>
            <p className="mt-1.5 mb-0 text-text-secondary text-[11px] leading-[1.55]">
              A automação organiza evidências, mostra limitações e mantém a decisão clínica com o médico responsável.
            </p>
          </article>
        </div>
      )}

      {/* Parâmetros */}
      {reportView === 'Parâmetros' && (
        <div className="grid mt-3.5 gap-2.5">
          {parameterBars.map((parameter) => (
            <div key={parameter.label} className="border border-border rounded-lg bg-surface-muted p-3.5">
              <div className="flex items-start justify-between gap-3">
                <div>
                  <strong className="block text-xs">{parameter.label}</strong>
                  <span className="block mt-1 text-text-secondary text-[10px]">{parameter.detail}</span>
                </div>
                <b className="text-text-primary font-display text-base whitespace-nowrap">{parameter.value}</b>
              </div>

              <div className="overflow-hidden h-[9px] mt-3 rounded-full bg-border">
                <span
                  className={clsx(
                    'block h-full rounded-full',
                    parameter.tone === 'success' ? 'bg-success' : 'bg-warning',
                  )}
                  style={{ width: `${parameter.percent}%` }}
                />
              </div>
            </div>
          ))}

          <div className="overflow-hidden mt-1 border border-border rounded-[11px]">
            {extractedData.map(([label, value, source]) => (
              <div key={label} className="grid items-center px-[15px] py-3 border-t border-border text-[11px] first:border-t-0" style={{ gridTemplateColumns: '1.4fr 1fr 0.8fr' }}>
                <strong className="font-semibold">{label}</strong>
                <span>{value}</span>
                <span className={clsx(
                  'inline-flex w-fit items-center gap-1.5 text-[10px] font-semibold',
                  label === 'ARTmax' ? 'text-warning' : 'text-success',
                )}>
                  {label === 'ARTmax' ? <AlertTriangle size={14} /> : <Check size={14} />}
                  {source}
                </span>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Protocolo */}
      {reportView === 'Protocolo' && (
        <div className="grid mt-3.5 gap-2.5">
          {protocolRules.map((rule, index) => (
            <div
              key={rule}
              className={clsx(
                'relative flex items-start gap-3 p-3.5 border rounded-lg bg-surface-muted',
                index === protocolRules.length - 1
                  ? 'border-[color-mix(in_srgb,var(--warning)_35%,var(--border))] bg-warning-soft'
                  : 'border-border',
              )}
            >
              <span className={clsx(
                'grid w-7 h-7 flex-none place-items-center rounded-full text-[11px] font-extrabold text-white',
                index === protocolRules.length - 1 ? 'bg-warning' : 'bg-primary',
              )}>
                {index + 1}
              </span>
              <div>
                <strong className="block text-xs">{rule}</strong>
                <p className="mt-1.5 mb-0 text-text-secondary text-[11px] leading-[1.55]">
                  {index === 3 ? 'A regra marca o relatório como parcial.' : 'Critério usado para compor a recomendação preliminar.'}
                </p>
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Relatório final */}
      <div className="mt-[18px] p-[18px] border border-primary-border rounded-xl bg-primary-soft">
        <div className="flex items-start justify-between gap-4 max-[580px]:flex-col max-[580px]:items-stretch">
          <div>
            <Eyebrow>RELATÓRIO PRELIMINAR</Eyebrow>
            <h2 className="mt-[5px] mb-0 font-display text-[18px] tracking-[-0.025em]">Síntese final</h2>
          </div>

          <div className="flex flex-wrap justify-end gap-2">
            <CaseStatus tone="success">Gerado automaticamente</CaseStatus>
            <CaseStatus tone={isReviewed ? 'success' : 'warning'}>
              {isReviewed ? 'Revisado pelo médico' : 'Aguardando revisão médica'}
            </CaseStatus>
          </div>
        </div>

        <div className="grid gap-1.5 mt-3.5 text-text-secondary text-xs leading-[1.7]">
          <strong className="text-text-primary text-[13px]">
            Maria da Silva, 38 anos: fluxo preliminar liberado para revisão.
          </strong>
          <span className="block">
            Sem alerta impeditivo nos parâmetros extraídos. ARTmax ausente; conclusões dependentes desse dado foram omitidas.
          </span>
        </div>

        <div className="flex items-center gap-[13px] mt-3.5">
          <PrimaryButton disabled={isReviewed} onClick={() => setIsReviewed(true)}>
            <Check size={17} />
            Revisar relatório
          </PrimaryButton>

          {isReviewed && (
            <span className="inline-flex items-center gap-[7px] text-success text-[11px] font-semibold">
              <CircleCheck size={16} />
              Revisão registrada neste protótipo.
            </span>
          )}
        </div>
      </div>
    </section>
  ) : (
    <section className="flex items-center justify-between gap-[18px] border border-border bg-surface rounded-2xl shadow-sm p-6 mt-5">
      <div>
        <Eyebrow>RELATÓRIOS</Eyebrow>
        <h2 className="mt-[5px] mb-0 font-display text-[17px] tracking-[-0.025em]">Nenhum relatório carregado nesta sessão</h2>
        <p className="mt-1.5 mb-0 max-w-[620px] text-text-secondary text-xs leading-[1.6]">
          Carregue o caso demonstrativo para gerar o relatório automatizado e enviá-lo para revisão médica posterior.
        </p>
      </div>

      <PrimaryButton onClick={loadDemoCase}>Carregar caso demonstrativo</PrimaryButton>
    </section>
  )

  const historySection = (
    <section className="border border-border bg-surface rounded-2xl shadow-sm p-6 mt-5">
      <div className="flex items-start justify-between gap-[18px]">
        <div>
          <Eyebrow>ATIVIDADE RECENTE</Eyebrow>
          <h2 className="mt-[5px] mb-0 font-display text-[17px] tracking-[-0.025em]">Histórico recente</h2>
        </div>
      </div>

      <div className="mt-4">
        {recentCases.map((item) => (
          <div
            key={item.patient}
            className="grid items-center py-[13px] border-t border-border first:border-t-0 gap-3"
            style={{ gridTemplateColumns: 'auto 1fr auto auto' }}
          >
            <span className="grid w-9 h-9 place-items-center rounded-[10px] bg-surface-muted text-primary text-[10px] font-bold">
              {item.initials}
            </span>

            <div className="flex min-w-0 flex-col">
              <strong className="overflow-hidden text-[11px] text-ellipsis whitespace-nowrap">{item.patient}</strong>
              <span className="mt-[3px] text-text-muted text-[9px]">Exame: {item.exam}</span>
            </div>

            <CaseStatus tone="success">Relatório: {item.report}</CaseStatus>
            <CaseStatus tone={item.statusClass as 'success' | 'warning'}>Revisão: {item.review}</CaseStatus>
          </div>
        ))}
      </div>
    </section>
  )

  return (
    <div className="grid min-h-screen grid-cols-[264px_minmax(0,1fr)] max-[820px]:grid-cols-1">
      {/* Sidebar */}
      <aside className="sidebar-bg sticky top-0 flex h-screen flex-col px-5 pb-[22px] pt-7 text-sidebar-text max-[820px]:static max-[820px]:h-auto max-[820px]:py-4">
        {/* Brand */}
        <div className="flex items-center gap-3 px-2 pb-[30px]">
          <div className="grid w-[42px] h-[42px] place-items-center rounded-[13px] border border-white/[0.12] bg-white/[0.09] text-[#79d4b7]">
            <Activity size={23} />
          </div>
          <div className="flex flex-col">
            <strong className="font-display text-[19px] tracking-[-0.02em]">RefratIA</strong>
            <span className="mt-0.5 text-sidebar-muted text-[11px]">Relatório refrativo preliminar</span>
          </div>
        </div>

        {/* Nav */}
        <nav className="flex flex-col gap-[5px] max-[820px]:hidden" aria-label="Navegação principal">
          <span className="mx-3 mb-2 mt-1 text-sidebar-muted text-[10px] font-bold tracking-[0.14em]">PLATAFORMA</span>

          {[
            { label: 'Visão geral', icon: <LayoutDashboard size={19} /> },
            { label: 'Nova análise', icon: <Plus size={19} /> },
            { label: 'Relatórios', icon: <FileText size={19} /> },
            { label: 'Histórico', icon: <History size={19} /> },
          ].map((item) => (
            <button
              key={item.label}
              className={clsx(
                'flex w-full items-center gap-3 px-[13px] py-[11px] border-0 rounded-[10px] text-sm font-medium text-left cursor-pointer transition-[background,color,transform] duration-[160ms] ease-[ease]',
                activeSection === item.label
                  ? 'bg-[rgb(103_205_171_/_14%)] text-[#9be0c9]'
                  : 'bg-transparent text-sidebar-muted hover:bg-white/[0.05] hover:text-sidebar-text hover:translate-x-0.5',
              )}
              onClick={() => setActiveSection(item.label)}
              type="button"
            >
              {item.icon}
              <span>{item.label}</span>
            </button>
          ))}

          <span className="mx-3 mb-2 mt-[26px] text-sidebar-muted text-[10px] font-bold tracking-[0.14em]">SISTEMA</span>

          <button className="flex w-full items-center gap-3 px-[13px] py-[11px] border-0 rounded-[10px] bg-transparent text-sidebar-muted text-sm font-medium text-left cursor-pointer transition-[background,color,transform] duration-[160ms] ease-[ease] hover:bg-white/[0.05] hover:text-sidebar-text hover:translate-x-0.5" type="button">
            <Settings size={19} />
            <span>Configurações</span>
          </button>
        </nav>

        {/* Footer */}
        <div className="mt-auto flex flex-col gap-[18px] max-[820px]:hidden">
          <div className="flex items-center gap-2.5 p-3 border border-white/[0.08] rounded-xl bg-white/[0.04]">
            <span className="grid w-[35px] h-[35px] flex-none place-items-center rounded-full bg-sidebar-secondary text-[#86d8bd]">
              <UserRound size={19} />
            </span>
            <div className="flex min-w-0 flex-col">
              <strong className="text-[13px]">Dr. Tiago</strong>
              <span className="overflow-hidden mt-0.5 text-sidebar-muted text-[10px] text-ellipsis whitespace-nowrap">Responsável clínico</span>
            </div>
          </div>

          <span className="ml-2.5 self-start text-sidebar-muted text-[10px] tracking-[0.06em] uppercase">Protótipo v0.1</span>
        </div>
      </aside>

      {/* Main */}
      <main className="min-w-0">
        {/* Topbar */}
        <header className="sticky top-0 z-50 isolate flex min-h-[90px] items-center justify-between gap-6 border-b border-border bg-[var(--surface)] px-[clamp(24px,4vw,56px)] py-5 shadow-sm">
          <div>
            <Eyebrow>PROTOCOLO DEMONSTRATIVO</Eyebrow>
            <h1 className="mt-1 mb-0 font-display text-2xl tracking-[-0.035em]">{activeSection}</h1>
          </div>

          <div className="flex items-center gap-3">
            <span className="hidden sm:flex items-center gap-2 px-3 py-2 border border-border rounded-full bg-surface text-text-secondary text-[11px] shadow-sm">
              <span className="w-[7px] h-[7px] rounded-full bg-warning shadow-[0_0_0_4px_var(--warning-soft)]" />
              Ambiente demonstrativo
            </span>

            <button
              aria-label={theme === 'dark' ? 'Ativar tema claro' : 'Ativar tema escuro'}
              className="grid w-[39px] h-[39px] place-items-center border border-border rounded-[11px] bg-surface cursor-pointer shadow-sm transition-[border-color,transform] duration-[160ms] ease-[ease] hover:border-border-strong hover:-translate-y-px"
              onClick={toggleTheme}
              type="button"
            >
              {theme === 'dark' ? <Sun size={19} /> : <Moon size={19} />}
            </button>
          </div>
        </header>

        {/* Page content */}
        <section className="w-full max-w-[1320px] mx-auto px-[clamp(24px,4vw,56px)] pt-8 pb-14">
          {/* Hero */}
          <div className="hero-bg flex items-center justify-between gap-6 p-6 border border-primary-border rounded-2xl max-[820px]:flex-col max-[820px]:items-start">
            <div className="flex items-start gap-[15px]">
              <span className="grid w-10 h-10 flex-none place-items-center rounded-xl bg-surface text-primary shadow-sm">
                <UploadCloud size={20} />
              </span>
              <div>
                <h2 className="m-0 font-display text-[18px] tracking-[-0.025em]">Envie exames e gere um relatório preliminar</h2>
                <p className="max-w-[690px] mt-1.5 mb-0 text-text-secondary text-[13px] leading-[1.6]">
                  O protótipo extrai dados automaticamente, aplica o Protocolo RefratIA v0.1 e disponibiliza o relatório antes da revisão médica posterior.
                </p>
              </div>
            </div>

            <div className="flex max-w-[290px] flex-none items-center gap-[9px] p-[10px_12px] border border-primary-border/70 rounded-[10px] bg-surface/[0.78] text-text-secondary text-[11px] leading-[1.45] max-[820px]:max-w-none">
              <ShieldCheck size={18} className="flex-none" />
              <span>Apoio à avaliação clínica. Não constitui diagnóstico, laudo ou indicação cirúrgica.</span>
            </div>
          </div>

          {/* Metrics */}
          <div className="grid grid-cols-4 gap-4 mt-5 max-[820px]:grid-cols-1">
            <MetricCard detail={metrics[0].detail} icon={<FileSearch size={20} />} label={metrics[0].label} value={metrics[0].value} />
            <MetricCard detail={metrics[1].detail} icon={<FileCheck size={20} />} label={metrics[1].label} tone={metrics[1].tone} value={metrics[1].value} />
            <MetricCard detail={metrics[2].detail} icon={<AlertTriangle size={20} />} label={metrics[2].label} tone={metrics[2].tone} value={metrics[2].value} />
            <MetricCard detail={metrics[3].detail} icon={<CircleCheck size={20} />} label={metrics[3].label} tone={metrics[3].tone} value={metrics[3].value} />
          </div>

          {/* Visão geral */}
          {activeSection === 'Visão geral' && (
            <>
              <section className="flex items-center justify-between gap-[18px] border border-border bg-surface rounded-2xl shadow-sm p-6 mt-5">
                <div>
                  <Eyebrow>RESUMO OPERACIONAL</Eyebrow>
                  <h2 className="mt-[5px] mb-0 font-display text-[17px] tracking-[-0.025em]">Fluxo automatizado ativo</h2>
                  <p className="mt-1.5 mb-0 max-w-[620px] text-text-secondary text-xs leading-[1.6]">
                    Exames entram no protótipo, geram relatório preliminar e seguem para revisão médica posterior.
                  </p>
                </div>
                <PrimaryButton onClick={() => setActiveSection('Nova análise')}>Nova análise</PrimaryButton>
              </section>
              {historySection}
            </>
          )}

          {/* Nova análise */}
          {activeSection === 'Nova análise' && (
            <>
              <div className="grid mt-5 gap-5 grid-cols-[minmax(0,1.6fr)_minmax(300px,0.7fr)] max-[1100px]:grid-cols-1">
                {/* Upload panel */}
                <section className="border border-border bg-surface rounded-2xl shadow-sm p-6">
                  <div className="flex items-start justify-between gap-[18px]">
                    <div>
                      <Eyebrow>NOVA ANÁLISE</Eyebrow>
                      <h2 className="mt-[5px] mb-0 font-display text-[17px] tracking-[-0.025em]">Enviar exames</h2>
                    </div>
                    <span className="px-[9px] py-1.5 rounded-[7px] bg-surface-muted text-text-secondary text-[10px]">
                      {reportGenerated ? 'Relatório disponível' : 'Aguardando exame'}
                    </span>
                  </div>

                  <div className="grid grid-cols-4 gap-2 mt-[25px] max-[580px]:grid-cols-1">
                    {steps.map((step, index) => (
                      <Step
                        key={step.title}
                        active={processingStep === index + 1}
                        completed={processingStep > index + 1}
                        description={step.description}
                        number={String(index + 1)}
                        title={step.title}
                      />
                    ))}
                  </div>

                  <button
                    className="grid w-full mt-5 items-center p-[18px] border border-dashed border-border-strong rounded-xl bg-surface-muted text-inherit text-left cursor-pointer transition-[border-color,background] hover:border-primary hover:bg-primary-soft/80"
                    style={{ gridTemplateColumns: 'auto 1fr auto' }}
                    onClick={loadDemoCase}
                    type="button"
                  >
                    <span className="grid w-[45px] h-[45px] place-items-center rounded-xl bg-primary-soft text-primary">
                      <UploadCloud size={27} />
                    </span>

                    <span className="mx-3.5">
                      <strong className="block text-xs">{selectedFile ?? 'Arraste os exames do paciente'}</strong>
                      <small className="block mt-1 text-text-muted text-[10px] leading-[1.45]">
                        {selectedFile
                          ? 'Exame demonstrativo recebido para extração automática.'
                          : 'Protótipo atual: modelo específico de Pentacam em PDF.'}
                      </small>
                    </span>

                    <span className="inline-flex items-center justify-center gap-2 px-[13px] py-[10px] border border-border rounded-[9px] bg-surface text-text-secondary text-[10px] font-semibold">
                      {selectedFile ? 'Recarregar caso' : 'Carregar caso demonstrativo'}
                    </span>
                  </button>

                  <div className="flex items-center justify-between gap-5 mt-5 max-[580px]:flex-col max-[580px]:items-stretch">
                    <div className="flex items-center gap-[7px] text-text-muted text-[10px]">
                      <ShieldCheck size={17} />
                      Use somente dados fictícios ou anonimizados neste protótipo.
                    </div>

                    {selectedFile && (
                      <span className="inline-flex items-center gap-[7px] px-[10px] py-[7px] border border-border rounded-full bg-surface-muted text-text-secondary text-[10px] font-semibold">
                        <FileText size={15} />
                        {selectedFile}
                      </span>
                    )}
                  </div>
                </section>

                {/* Protocol panel */}
                <aside className="protocol-panel-bg border border-border rounded-2xl shadow-sm p-6">
                  <div className="flex items-start justify-between gap-[18px]">
                    <div>
                      <Eyebrow>PROTOCOLO ATIVO</Eyebrow>
                      <h2 className="mt-[5px] mb-0 font-display text-[17px] tracking-[-0.025em]">Protocolo RefratIA v0.1</h2>
                    </div>
                    <Stethoscope size={22} className="text-primary" />
                  </div>

                  <div className="grid mt-5 gap-2">
                    {['18 a 40 anos', '41 a 55 anos', '56 anos ou mais'].map((band) => (
                      <span
                        key={band}
                        className={clsx(
                          'flex items-center gap-[7px] px-[10px] py-[9px] border rounded-[9px] text-[10px] font-semibold',
                          band === '18 a 40 anos'
                            ? 'border-primary-border bg-primary-soft text-primary'
                            : 'border-border bg-surface text-text-secondary',
                        )}
                      >
                        {band === '18 a 40 anos' && <Check size={14} />}
                        {band}
                      </span>
                    ))}
                  </div>

                  <div className="flex flex-col gap-2 mt-[22px]">
                    {protocolRules.map((rule, index) => (
                      <div key={rule} className="flex items-start gap-3 py-[13px] border-b border-border last:border-b-0">
                        <span className="text-primary font-display text-[10px] font-extrabold">
                          {String(index + 1).padStart(2, '0')}
                        </span>
                        <div>
                          <strong className="block text-[11px]">{rule}</strong>
                          <p className="mt-1 mb-0 text-text-secondary text-[10px] leading-[1.5]">
                            Regra demonstrativa acionada para rastreabilidade do relatório preliminar.
                          </p>
                        </div>
                      </div>
                    ))}
                  </div>

                  <div className="mt-5 p-[15px] border border-primary-border rounded-[11px] bg-primary-soft">
                    <span className="text-primary text-[9px] font-bold tracking-[0.12em]">ESCOPO ATUAL</span>
                    <ul className="flex flex-col gap-[9px] mt-3 mb-0 p-0 list-none">
                      {['Protocolo estruturado e demonstrativo', 'Relatório automatizado preliminar', 'Revisão médica posterior'].map((item) => (
                        <li key={item} className="flex items-center gap-2 text-text-secondary text-[10px]">
                          <Check size={15} className="text-success flex-none" />
                          {item}
                        </li>
                      ))}
                    </ul>
                  </div>
                </aside>
              </div>

              {reportGenerated && reportSection}
            </>
          )}

          {activeSection === 'Relatórios' && reportSection}
          {activeSection === 'Histórico' && historySection}
        </section>
      </main>
    </div>
  )
}

export default App
