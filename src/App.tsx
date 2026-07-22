import { useEffect, useState, type ReactNode } from 'react'
import {
  Activity,
  AlertTriangle,
  ArrowRight,
  Check,
  ChevronRight,
  CircleCheck,
  FileSearch,
  FileText,
  History,
  LayoutDashboard,
  Moon,
  Plus,
  Settings,
  ShieldCheck,
  Sparkles,
  Stethoscope,
  Sun,
  UploadCloud,
  UserRound,
} from 'lucide-react'

type Theme = 'light' | 'dark'

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

const metrics = [
  {
    label: 'Casos analisados',
    value: '24',
    detail: '+6 nesta semana',
  },
  {
    label: 'Revisões pendentes',
    value: '3',
    detail: 'Requerem confirmação',
    tone: 'warning' as const,
  },
  {
    label: 'Relatórios concluídos',
    value: '21',
    detail: '87,5% dos casos',
    tone: 'success' as const,
  },
]

const recentCases = [
  {
    initials: 'MS',
    patient: 'Caso demonstrativo 024',
    date: 'Hoje, 14:32',
    status: 'Revisão',
    statusClass: 'warning',
  },
  {
    initials: 'AR',
    patient: 'Caso demonstrativo 023',
    date: 'Hoje, 10:18',
    status: 'Concluído',
    statusClass: 'success',
  },
  {
    initials: 'LC',
    patient: 'Caso demonstrativo 022',
    date: 'Ontem, 16:45',
    status: 'Concluído',
    statusClass: 'success',
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
  const [activeSection, setActiveSection] = useState('Nova avaliação')
  const [showDemoResult, setShowDemoResult] = useState(false)

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
    setSelectedFile('pentacam-caso-demonstrativo.pdf')
    setShowDemoResult(true)
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
            <span>Pré-avaliação refrativa</span>
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
              label: 'Nova avaliação',
              icon: <Plus size={19} />,
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
              <span>Cirurgião responsável</span>
            </div>
          </div>

          <span className="prototype-badge">Protótipo v0.1</span>
        </div>
      </aside>

      <main className="main-content">
        <header className="topbar">
          <div>
            <span className="eyebrow">ASSISTENTE CLÍNICO EXPERIMENTAL</span>
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
                <Sparkles size={20} />
              </span>

              <div>
                <h2>Transforme exames em uma avaliação estruturada</h2>
                <p>
                  Envie os dados do caso, confirme os parâmetros identificados
                  e gere um relatório rastreável para revisão médica.
                </p>
              </div>
            </div>

            <div className="hero__notice">
              <ShieldCheck size={18} />
              <span>
                O sistema apoia a avaliação e não substitui a decisão clínica.
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
              icon={<AlertTriangle size={20} />}
              label={metrics[1].label}
              tone={metrics[1].tone}
              value={metrics[1].value}
            />

            <MetricCard
              detail={metrics[2].detail}
              icon={<CircleCheck size={20} />}
              label={metrics[2].label}
              tone={metrics[2].tone}
              value={metrics[2].value}
            />
          </div>

          <div className="workspace-grid">
            <section className="panel new-analysis-panel">
              <div className="panel__header">
                <div>
                  <span className="panel__eyebrow">NOVO CASO</span>
                  <h2>Iniciar pré-avaliação</h2>
                </div>

                <span className="step-counter">Etapa 1 de 3</span>
              </div>

              <div className="analysis-steps">
                <Step
                  active
                  description="Identificação e refração"
                  number="1"
                  title="Dados do caso"
                />
                <Step
                  description="Pentacam suportado"
                  number="2"
                  title="Enviar exame"
                />
                <Step
                  description="Confirmar parâmetros"
                  number="3"
                  title="Revisão médica"
                />
              </div>

              <div className="form-grid">
                <label className="field">
                  <span>Identificação do caso</span>
                  <input
                    defaultValue="Caso demonstrativo 025"
                    placeholder="Ex.: Caso 025"
                  />
                </label>

                <label className="field">
                  <span>Idade</span>
                  <div className="input-with-suffix">
                    <input defaultValue="38" type="number" />
                    <span>anos</span>
                  </div>
                </label>

                <label className="field">
                  <span>Esfera</span>
                  <div className="input-with-suffix">
                    <input defaultValue="-4.50" step="0.25" type="number" />
                    <span>D</span>
                  </div>
                </label>

                <label className="field">
                  <span>Cilindro</span>
                  <div className="input-with-suffix">
                    <input defaultValue="-1.25" step="0.25" type="number" />
                    <span>D</span>
                  </div>
                </label>
              </div>

              <div className="upload-area">
                <span className="upload-area__icon">
                  <UploadCloud size={27} />
                </span>

                <div>
                  <strong>
                    {selectedFile ?? 'Envie o relatório do Pentacam'}
                  </strong>
                  <p>
                    Modelo demonstrativo suportado em PDF, com tamanho máximo
                    de 10 MB.
                  </p>
                </div>

                <button
                  className="secondary-button"
                  onClick={loadDemoCase}
                  type="button"
                >
                  {selectedFile ? 'Arquivo carregado' : 'Usar caso demonstrativo'}
                </button>
              </div>

              <div className="panel__footer">
                <div className="privacy-note">
                  <ShieldCheck size={17} />
                  Use somente dados fictícios ou anonimizados neste protótipo.
                </div>

                <button
                  className="primary-button"
                  disabled={!selectedFile}
                  onClick={() => setShowDemoResult(true)}
                  type="button"
                >
                  Processar exame
                  <ArrowRight size={18} />
                </button>
              </div>
            </section>

            <aside className="panel protocol-panel">
              <div className="panel__header">
                <div>
                  <span className="panel__eyebrow">PROTOCOLO ATIVO</span>
                  <h2>Fluxo do protótipo</h2>
                </div>

                <Stethoscope size={22} />
              </div>

              <div className="protocol-list">
                <div className="protocol-item">
                  <span>01</span>
                  <div>
                    <strong>Extração assistida</strong>
                    <p>
                      O sistema identifica parâmetros candidatos no exame.
                    </p>
                  </div>
                </div>

                <div className="protocol-item">
                  <span>02</span>
                  <div>
                    <strong>Confirmação humana</strong>
                    <p>
                      Nenhum valor é utilizado antes da revisão do médico.
                    </p>
                  </div>
                </div>

                <div className="protocol-item">
                  <span>03</span>
                  <div>
                    <strong>Regras rastreáveis</strong>
                    <p>
                      O relatório informa quais critérios foram aplicados.
                    </p>
                  </div>
                </div>
              </div>

              <div className="protocol-scope">
                <span>ESCOPO ATUAL</span>

                <ul>
                  <li>
                    <Check size={15} />
                    Um modelo de Pentacam
                  </li>
                  <li>
                    <Check size={15} />
                    Cinco parâmetros principais
                  </li>
                  <li>
                    <Check size={15} />
                    Direcionamento genérico
                  </li>
                  <li>
                    <Check size={15} />
                    Relatório para revisão
                  </li>
                </ul>
              </div>
            </aside>
          </div>

          {showDemoResult && (
            <section className="panel result-panel">
              <div className="panel__header">
                <div>
                  <span className="panel__eyebrow">RESULTADO DEMONSTRATIVO</span>
                  <h2>Parâmetros identificados</h2>
                </div>

                <span className="result-status">
                  <AlertTriangle size={16} />
                  Revisão necessária
                </span>
              </div>

              <div className="parameters-table">
                <div className="parameters-table__header">
                  <span>Parâmetro</span>
                  <span>Valor identificado</span>
                  <span>Confirmação</span>
                </div>

                {[
                  ['Kmax', '44,2 D', 'Confirmado'],
                  ['Paquimetria mínima', '521 µm', 'Confirmado'],
                  ['BAD-D', '1,18', 'Confirmado'],
                  ['ARTmax', 'Não identificado', 'Revisar'],
                  ['Astigmatismo corneano', '1,31 D', 'Confirmado'],
                ].map(([parameter, value, status]) => (
                  <div className="parameters-table__row" key={parameter}>
                    <strong>{parameter}</strong>
                    <span>{value}</span>
                    <span
                      className={
                        status === 'Confirmado'
                          ? 'parameter-status parameter-status--confirmed'
                          : 'parameter-status parameter-status--review'
                      }
                    >
                      {status === 'Confirmado' ? (
                        <Check size={14} />
                      ) : (
                        <AlertTriangle size={14} />
                      )}
                      {status}
                    </span>
                  </div>
                ))}
              </div>

              <div className="result-summary">
                <FileText size={21} />

                <div>
                  <strong>Prévia do direcionamento</strong>
                  <p>
                    O caso encontra-se na faixa etária em que abordagens
                    corneanas podem ser consideradas, desde que os critérios de
                    segurança sejam confirmados. O astigmatismo corneano
                    identificado aciona a regra de avaliação de tratamento
                    personalizado. O ARTmax precisa ser revisado antes da
                    emissão do relatório.
                  </p>
                </div>

                <button className="text-button" type="button">
                  Ver regras aplicadas
                  <ChevronRight size={17} />
                </button>
              </div>
            </section>
          )}

          <section className="panel recent-panel">
            <div className="panel__header">
              <div>
                <span className="panel__eyebrow">ATIVIDADE RECENTE</span>
                <h2>Últimos casos</h2>
              </div>

              <button className="text-button" type="button">
                Ver histórico
                <ChevronRight size={17} />
              </button>
            </div>

            <div className="recent-list">
              {recentCases.map((item) => (
                <div className="recent-case" key={item.patient}>
                  <span className="recent-case__avatar">{item.initials}</span>

                  <div className="recent-case__identity">
                    <strong>{item.patient}</strong>
                    <span>{item.date}</span>
                  </div>

                  <span
                    className={`case-status case-status--${item.statusClass}`}
                  >
                    {item.status}
                  </span>

                  <button
                    aria-label={`Abrir ${item.patient}`}
                    className="icon-button"
                    type="button"
                  >
                    <ChevronRight size={18} />
                  </button>
                </div>
              ))}
            </div>
          </section>
        </section>
      </main>
    </div>
  )
}

export default App
