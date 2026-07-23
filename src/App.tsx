import { useEffect, useState, type ReactNode } from 'react'
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
  {
    title: 'Enviar exames',
    description: 'PDF demonstrativo',
  },
  {
    title: 'Extrair dados',
    description: 'Nome, idade e parâmetros',
  },
  {
    title: 'Aplicar protocolo',
    description: 'RefratIA v0.1',
  },
  {
    title: 'Gerar relatório',
    description: 'Preliminar automatizado',
  },
]

const metrics = [
  {
    label: 'Exames processados',
    value: '24',
    detail: 'Pentacam demonstrativo',
  },
  {
    label: 'Relatórios gerados',
    value: '24',
    detail: 'Geração automatizada',
    tone: 'success' as const,
  },
  {
    label: 'Aguardando revisão',
    value: '3',
    detail: 'Revisão médica posterior',
    tone: 'warning' as const,
  },
  {
    label: 'Com alerta de extração',
    value: '1',
    detail: 'Dado ausente não impeditivo',
    tone: 'warning' as const,
  },
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
  {
    label: 'Paquimetria mínima',
    value: '521 µm',
    percent: 74,
    detail: 'Boa espessura no caso demonstrativo',
    tone: 'success',
  },
  {
    label: 'Kmax',
    value: '44,2 D',
    percent: 58,
    detail: 'Curvatura sem gatilho crítico',
    tone: 'success',
  },
  {
    label: 'BAD-D',
    value: '1,18',
    percent: 42,
    detail: 'Compatível com fluxo preliminar',
    tone: 'success',
  },
  {
    label: 'ARTmax',
    value: 'Não identificado',
    percent: 18,
    detail: 'Bloqueia conclusões dependentes',
    tone: 'warning',
  },
]

const reportViews: ReportView[] = ['Resumo', 'Parâmetros', 'Protocolo']

const recentCases = [
  {
    initials: 'MS',
    patient: 'Maria da Silva',
    exam: 'Pentacam',
    report: 'Gerado',
    review: 'Pendente',
    statusClass: 'warning',
  },
  {
    initials: 'JL',
    patient: 'João Lima',
    exam: 'Pentacam',
    report: 'Gerado',
    review: 'Revisado',
    statusClass: 'success',
  },
  {
    initials: 'AR',
    patient: 'Ana Rocha',
    exam: 'Pentacam',
    report: 'Parcial',
    review: 'Atenção',
    statusClass: 'warning',
  },
]

function getInitialTheme(): Theme {
  const storedTheme = localStorage.getItem('refratia-theme')

  if (storedTheme === 'light' || storedTheme === 'dark') {
    return storedTheme
  }

  return window.matchMedia('(prefers-color-scheme: dark)').matches
    ? 'dark'
    : 'light'
}

function MetricCard({
  icon,
  label,
  value,
  detail,
  tone = 'default',
}: MetricCardProps) {
  return (
    <article className={`metric-card metric-card--${tone}`}>
      <div className="metric-card__header">
        <span className="metric-card__icon">{icon}</span>
        <span className="metric-card__label">{label}</span>
      </div>

      <strong className="metric-card__value">{value}</strong>
      <span className="metric-card__detail">{detail}</span>
    </article>
  )
}

function Step({
  number,
  title,
  description,
  active = false,
  completed = false,
}: StepProps) {
  return (
    <div
      className={[
        'analysis-step',
        active ? 'analysis-step--active' : '',
        completed ? 'analysis-step--completed' : '',
      ]
        .filter(Boolean)
        .join(' ')}
    >
      <span className="analysis-step__number">
        {completed ? <Check size={15} strokeWidth={3} /> : number}
      </span>

      <div>
        <strong>{title}</strong>
        <p>{description}</p>
      </div>
    </div>
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
  const reportSection = reportGenerated ? (
    <section className="panel result-panel">
      <div className="panel__header">
        <div>
          <span className="panel__eyebrow">RESULTADO DEMONSTRATIVO</span>
          <h2>Relatório clínico interativo</h2>
        </div>

        <span className="result-status">
          <AlertTriangle size={16} />
          ARTmax exige atenção
        </span>
      </div>

      <div className="report-dashboard">
        <div className="report-score">
          <span className="report-score__ring">74</span>
          <div>
            <span className="panel__eyebrow">SCORE PRELIMINAR</span>
            <h3>Candidato para revisão médica</h3>
            <p>
              Parâmetros principais disponíveis, com uma pendência que limita
              conclusões dependentes de ARTmax.
            </p>
          </div>
        </div>

        <div className="report-highlights">
          {reportHighlights.map(([value, label, detail]) => (
            <div className="report-highlight" key={label}>
              <strong>{value}</strong>
              <span>{label}</span>
              <small>{detail}</small>
            </div>
          ))}
        </div>
      </div>

      <div className="report-tabs" role="tablist" aria-label="Visões do relatório">
        {reportViews.map((view) => (
          <button
            aria-selected={reportView === view}
            className={
              reportView === view
                ? 'report-tab report-tab--active'
                : 'report-tab'
            }
            key={view}
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

      {reportView === 'Resumo' && (
        <div className="report-summary-grid">
          <article className="report-insight report-insight--success">
            <CircleCheck size={20} />
            <div>
              <span>Achado principal</span>
              <strong>Sem alerta impeditivo nos dados identificados</strong>
              <p>
                Idade, Kmax, paquimetria e BAD-D mantêm o caso no fluxo
                preliminar para revisão.
              </p>
            </div>
          </article>

          <article className="report-insight report-insight--warning">
            <AlertTriangle size={20} />
            <div>
              <span>Pendência</span>
              <strong>ARTmax não foi extraído do PDF</strong>
              <p>
                O relatório omite qualquer conclusão que dependa desse
                parâmetro.
              </p>
            </div>
          </article>

          <article className="decision-card">
            <span className="panel__eyebrow">CONDUTA DO SISTEMA</span>
            <h3>Gerar relatório parcial e exigir revisão</h3>
            <p>
              A automação organiza evidências, mostra limitações e mantém a
              decisão clínica com o médico responsável.
            </p>
          </article>
        </div>
      )}

      {reportView === 'Parâmetros' && (
        <div className="parameter-bars">
          {parameterBars.map((parameter) => (
            <div className="parameter-bar" key={parameter.label}>
              <div className="parameter-bar__header">
                <div>
                  <strong>{parameter.label}</strong>
                  <span>{parameter.detail}</span>
                </div>
                <b>{parameter.value}</b>
              </div>

              <div className="parameter-bar__track">
                <span
                  className={`parameter-bar__fill parameter-bar__fill--${parameter.tone}`}
                  style={{ width: `${parameter.percent}%` }}
                />
              </div>
            </div>
          ))}

          <div className="parameters-table parameters-table--compact">
            {extractedData.map(([label, value, source]) => (
              <div className="parameters-table__row" key={label}>
                <strong>{label}</strong>
                <span>{value}</span>
                <span
                  className={
                    label === 'ARTmax'
                      ? 'parameter-status parameter-status--review'
                      : 'parameter-status parameter-status--confirmed'
                  }
                >
                  {label === 'ARTmax' ? (
                    <AlertTriangle size={14} />
                  ) : (
                    <Check size={14} />
                  )}
                  {source}
                </span>
              </div>
            ))}
          </div>
        </div>
      )}

      {reportView === 'Protocolo' && (
        <div className="decision-timeline">
          {protocolRules.map((rule, index) => (
            <div className="decision-step" key={rule}>
              <span>{index + 1}</span>
              <div>
                <strong>{rule}</strong>
                <p>
                  {index === 3
                    ? 'A regra marca o relatório como parcial.'
                    : 'Critério usado para compor a recomendação preliminar.'}
                </p>
              </div>
            </div>
          ))}
        </div>
      )}

      <div className="report-card">
        <div className="report-card__header">
          <div>
            <span className="panel__eyebrow">RELATÓRIO PRELIMINAR</span>
            <h2>Síntese final</h2>
          </div>

          <div className="report-statuses">
            <span className="case-status case-status--success">
              Gerado automaticamente
            </span>
            <span
              className={
                isReviewed
                  ? 'case-status case-status--success'
                  : 'case-status case-status--warning'
              }
            >
              {isReviewed ? 'Revisado pelo médico' : 'Aguardando revisão médica'}
            </span>
          </div>
        </div>

        <div className="report-text">
          <strong>
            Maria da Silva, 38 anos: fluxo preliminar liberado para revisão.
          </strong>
          <span>
            Sem alerta impeditivo nos parâmetros extraídos. ARTmax ausente;
            conclusões dependentes desse dado foram omitidas.
          </span>
        </div>

        <div className="report-actions">
          <button
            className="primary-button"
            disabled={isReviewed}
            onClick={() => setIsReviewed(true)}
            type="button"
          >
            <Check size={17} />
            Revisar relatório
          </button>

          {isReviewed && (
            <span className="review-feedback">
              <CircleCheck size={16} />
              Revisão registrada neste protótipo.
            </span>
          )}
        </div>
      </div>
    </section>
  ) : (
    <section className="panel result-panel empty-panel">
      <div>
        <span className="panel__eyebrow">RELATÓRIOS</span>
        <h2>Nenhum relatório carregado nesta sessão</h2>
        <p>
          Carregue o caso demonstrativo para gerar o relatório automatizado e
          enviá-lo para revisão médica posterior.
        </p>
      </div>

      <button className="primary-button" onClick={loadDemoCase} type="button">
        Carregar caso demonstrativo
      </button>
    </section>
  )

  const historySection = (
    <section className="panel recent-panel">
      <div className="panel__header">
        <div>
          <span className="panel__eyebrow">ATIVIDADE RECENTE</span>
          <h2>Histórico recente</h2>
        </div>
      </div>

      <div className="recent-list">
        {recentCases.map((item) => (
          <div className="recent-case" key={item.patient}>
            <span className="recent-case__avatar">{item.initials}</span>

            <div className="recent-case__identity">
              <strong>{item.patient}</strong>
              <span>Exame: {item.exam}</span>
            </div>

            <span className="case-status case-status--success">
              Relatório: {item.report}
            </span>

            <span className={`case-status case-status--${item.statusClass}`}>
              Revisão: {item.review}
            </span>
          </div>
        ))}
      </div>
    </section>
  )

  useEffect(() => {
    document.documentElement.dataset.theme = theme
    localStorage.setItem('refratia-theme', theme)
  }, [theme])

  function toggleTheme() {
    setTheme((currentTheme) =>
      currentTheme === 'dark' ? 'light' : 'dark',
    )
  }

  function loadDemoCase() {
    setSelectedFile('pentacam-maria-silva.pdf')
    setIsReviewed(false)
    setProcessingStep(1)

    steps.slice(1).forEach((_, index) => {
      window.setTimeout(() => {
        setProcessingStep(index + 2)
      }, 180 * (index + 1))
    })
  }

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <div className="brand">
          <div className="brand__symbol">
            <Activity size={23} />
          </div>

          <div>
            <strong>RefratIA</strong>
            <span>Relatório refrativo preliminar</span>
          </div>
        </div>

        <nav className="sidebar__navigation" aria-label="Navegação principal">
          <span className="navigation-label">PLATAFORMA</span>

          {[
            {
              label: 'Visão geral',
              icon: <LayoutDashboard size={19} />,
            },
            {
              label: 'Nova análise',
              icon: <Plus size={19} />,
            },
            {
              label: 'Relatórios',
              icon: <FileText size={19} />,
            },
            {
              label: 'Histórico',
              icon: <History size={19} />,
            },
          ].map((item) => (
            <button
              className={
                activeSection === item.label
                  ? 'navigation-item navigation-item--active'
                  : 'navigation-item'
              }
              key={item.label}
              onClick={() => setActiveSection(item.label)}
              type="button"
            >
              {item.icon}
              <span>{item.label}</span>
            </button>
          ))}

          <span className="navigation-label navigation-label--secondary">
            SISTEMA
          </span>

          <button className="navigation-item" type="button">
            <Settings size={19} />
            <span>Configurações</span>
          </button>
        </nav>

        <div className="sidebar__footer">
          <div className="doctor-card">
            <span className="doctor-card__avatar">
              <UserRound size={19} />
            </span>

            <div>
              <strong>Dr. Tiago</strong>
              <span>Responsável clínico</span>
            </div>
          </div>

          <span className="prototype-badge">Protótipo v0.1</span>
        </div>
      </aside>

      <main className="main-content">
        <header className="topbar">
          <div>
            <span className="eyebrow">PROTOCOLO DEMONSTRATIVO</span>
            <h1>{activeSection}</h1>
          </div>

          <div className="topbar__actions">
            <span className="environment-badge">
              <span />
              Ambiente demonstrativo
            </span>

            <button
              aria-label={
                theme === 'dark'
                  ? 'Ativar tema claro'
                  : 'Ativar tema escuro'
              }
              className="theme-toggle"
              onClick={toggleTheme}
              type="button"
            >
              {theme === 'dark' ? <Sun size={19} /> : <Moon size={19} />}
            </button>
          </div>
        </header>

        <section className="page-content">
          <div className="hero">
            <div className="hero__copy">
              <span className="hero__icon">
                <UploadCloud size={20} />
              </span>

              <div>
                <h2>Envie exames e gere um relatório preliminar</h2>
                <p>
                  O protótipo extrai dados automaticamente, aplica o Protocolo
                  RefratIA v0.1 e disponibiliza o relatório antes da revisão
                  médica posterior.
                </p>
              </div>
            </div>

            <div className="hero__notice">
              <ShieldCheck size={18} />
              <span>
                Apoio à avaliação clínica. Não constitui diagnóstico, laudo ou
                indicação cirúrgica.
              </span>
            </div>
          </div>

          <div className="metrics-grid">
            <MetricCard
              detail={metrics[0].detail}
              icon={<FileSearch size={20} />}
              label={metrics[0].label}
              value={metrics[0].value}
            />

            <MetricCard
              detail={metrics[1].detail}
              icon={<FileCheck size={20} />}
              label={metrics[1].label}
              tone={metrics[1].tone}
              value={metrics[1].value}
            />

            <MetricCard
              detail={metrics[2].detail}
              icon={<AlertTriangle size={20} />}
              label={metrics[2].label}
              tone={metrics[2].tone}
              value={metrics[2].value}
            />

            <MetricCard
              detail={metrics[3].detail}
              icon={<CircleCheck size={20} />}
              label={metrics[3].label}
              tone={metrics[3].tone}
              value={metrics[3].value}
            />
          </div>

          {activeSection === 'Visão geral' && (
            <>
              <section className="panel result-panel empty-panel">
                <div>
                  <span className="panel__eyebrow">RESUMO OPERACIONAL</span>
                  <h2>Fluxo automatizado ativo</h2>
                  <p>
                    Exames entram no protótipo, geram relatório preliminar e
                    seguem para revisão médica posterior.
                  </p>
                </div>

                <button
                  className="primary-button"
                  onClick={() => setActiveSection('Nova análise')}
                  type="button"
                >
                  Nova análise
                </button>
              </section>

              {historySection}
            </>
          )}

          {activeSection === 'Nova análise' && (
            <>
              <div className="workspace-grid">
                <section className="panel new-analysis-panel">
                  <div className="panel__header">
                    <div>
                      <span className="panel__eyebrow">NOVA ANÁLISE</span>
                      <h2>Enviar exames</h2>
                    </div>

                    <span className="step-counter">
                      {reportGenerated
                        ? 'Relatório disponível'
                        : 'Aguardando exame'}
                    </span>
                  </div>

                  <div className="analysis-steps">
                    {steps.map((step, index) => (
                      <Step
                        active={processingStep === index + 1}
                        completed={processingStep > index + 1}
                        description={step.description}
                        key={step.title}
                        number={String(index + 1)}
                        title={step.title}
                      />
                    ))}
                  </div>

                  <button
                    className="upload-area upload-area--button"
                    onClick={loadDemoCase}
                    type="button"
                  >
                    <span className="upload-area__icon">
                      <UploadCloud size={27} />
                    </span>

                    <span>
                      <strong>
                        {selectedFile ?? 'Arraste os exames do paciente'}
                      </strong>
                      <small>
                        {selectedFile
                          ? 'Exame demonstrativo recebido para extração automática.'
                          : 'Protótipo atual: modelo específico de Pentacam em PDF.'}
                      </small>
                    </span>

                    <span className="secondary-button">
                      {selectedFile
                        ? 'Recarregar caso'
                        : 'Carregar caso demonstrativo'}
                    </span>
                  </button>

                  <div className="panel__footer">
                    <div className="privacy-note">
                      <ShieldCheck size={17} />
                      Use somente dados fictícios ou anonimizados neste
                      protótipo.
                    </div>

                    {selectedFile && (
                      <span className="file-chip">
                        <FileText size={15} />
                        {selectedFile}
                      </span>
                    )}
                  </div>
                </section>

                <aside className="panel protocol-panel">
                  <div className="panel__header">
                    <div>
                      <span className="panel__eyebrow">PROTOCOLO ATIVO</span>
                      <h2>Protocolo RefratIA v0.1</h2>
                    </div>

                    <Stethoscope size={22} />
                  </div>

                  <div className="age-bands">
                    {['18 a 40 anos', '41 a 55 anos', '56 anos ou mais'].map(
                      (band) => (
                        <span
                          className={
                            band === '18 a 40 anos'
                              ? 'age-band age-band--selected'
                              : 'age-band'
                          }
                          key={band}
                        >
                          {band === '18 a 40 anos' && <Check size={14} />}
                          {band}
                        </span>
                      ),
                    )}
                  </div>

                  <div className="protocol-list">
                    {protocolRules.map((rule, index) => (
                      <div className="protocol-item" key={rule}>
                        <span>{String(index + 1).padStart(2, '0')}</span>
                        <div>
                          <strong>{rule}</strong>
                          <p>
                            Regra demonstrativa acionada para rastreabilidade do
                            relatório preliminar.
                          </p>
                        </div>
                      </div>
                    ))}
                  </div>

                  <div className="protocol-scope">
                    <span>ESCOPO ATUAL</span>

                    <ul>
                      <li>
                        <Check size={15} />
                        Protocolo estruturado e demonstrativo
                      </li>
                      <li>
                        <Check size={15} />
                        Relatório automatizado preliminar
                      </li>
                      <li>
                        <Check size={15} />
                        Revisão médica posterior
                      </li>
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
