import { useEffect, useRef, useState } from 'react'
import clsx from 'clsx'
import { Activity, Moon, Sun, UserRound } from 'lucide-react'
import BenchmarkOcrPage from '../features/benchmark/BenchmarkOcrPage'
import NewAnalysisPage from '../features/intake/NewAnalysisPage'
import ProtocolPage from '../features/protocol/ProtocolPage'
import ReportsPage from '../features/reports/ReportsPage'
import { BUNDLED_REPORT_ID } from '../features/reports/reportModel'
import RoadmapPage from '../features/roadmap/RoadmapPage'
import SavedExamsPage from '../features/saved-exams/SavedExamsPage'
import { useCaseList } from '../shared/cases'
import { Eyebrow, PrimaryButton } from '../shared/ui'
import { sections, useRoute, type Section } from './routes'

type Theme = 'light' | 'dark'

function useTheme() {
  const [theme, setTheme] = useState<Theme>(() => {
    const stored = localStorage.getItem('refratia-theme')
    if (stored === 'light' || stored === 'dark') return stored
    return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
  })

  useEffect(() => {
    document.documentElement.dataset.theme = theme
    localStorage.setItem('refratia-theme', theme)
  }, [theme])

  return [theme, () => setTheme((current) => current === 'dark' ? 'light' : 'dark')] as const
}

function useTemporarilyHidden(ms: number) {
  const [hidden, setHidden] = useState(false)
  const timer = useRef<number | null>(null)

  useEffect(() => () => {
    if (timer.current !== null) window.clearTimeout(timer.current)
  }, [])

  function hide() {
    setHidden(true)
    if (timer.current !== null) window.clearTimeout(timer.current)
    timer.current = window.setTimeout(() => {
      setHidden(false)
      timer.current = null
    }, ms)
  }

  return [hidden, hide] as const
}

function Sidebar({ active, onSelect }: { active: Section; onSelect: (section: Section) => void }) {
  return (
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
        {sections.map((section, index) => (
          <div className="contents" key={section.id}>
            {sections[index - 1]?.group !== section.group && (
              <span className={clsx('mx-3 mb-2 text-xs font-bold tracking-[0.14em] text-sidebar-muted', index === 0 ? 'mt-1' : 'mt-6')}>{section.group}</span>
            )}
            <button
              aria-current={active.id === section.id ? 'page' : undefined}
              className={clsx(
                'flex w-full items-center gap-3 rounded-[10px] border-0 px-[13px] py-[11px] text-left text-sm font-medium',
                active.id === section.id ? 'bg-[rgb(103_205_171_/_14%)] text-[#9be0c9]' : 'bg-transparent text-sidebar-muted hover:bg-white/[0.05] hover:text-sidebar-text',
              )}
              onClick={() => onSelect(section)}
              type="button"
            >
              <section.icon size={19} />{section.label}
            </button>
          </div>
        ))}
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
  )
}

export default function App() {
  const [theme, toggleTheme] = useTheme()
  const [headerHidden, hideHeader] = useTemporarilyHidden(10_000)
  const route = useRoute()
  const caseList = useCaseList()
  const { section, reportId } = route

  return (
    <div className="grid min-h-screen grid-cols-[264px_minmax(0,1fr)] max-[820px]:grid-cols-1">
      <Sidebar active={section} onSelect={(item) => route.goTo(item.id)} />

      <main className="min-w-0">
        {!headerHidden && <header className="sticky top-0 z-50 flex min-h-[90px] items-center justify-between gap-6 border-b border-border bg-surface px-[clamp(16px,4vw,56px)] py-5 shadow-sm">
          <div>
            <Eyebrow>{section.id === 'protocol' ? 'DOCUMENTO CLÍNICO' : 'PROTOCOLO DEMONSTRATIVO'}</Eyebrow>
            <h1 className="mb-0 mt-1 font-display text-2xl tracking-[-0.035em]">{section.label}</h1>
          </div>
          <div className="flex items-center gap-3">
            {reportId === BUNDLED_REPORT_ID ? (
              <span className="hidden items-center gap-2 rounded-full border border-border bg-surface px-3 py-2 text-sm text-text-secondary sm:flex">
                <span className="h-[7px] w-[7px] rounded-full bg-danger" />
                Caso real
              </span>
            ) : (
              <button
                aria-label="Ocultar a navegação por 10 segundos"
                className="hidden items-center gap-2 rounded-full border border-border bg-surface px-3 py-2 text-sm text-text-secondary hover:border-border-strong sm:flex"
                onClick={hideHeader}
                title="Ocultar navegação por 10 segundos"
                type="button"
              >
                <span className="h-[7px] w-[7px] rounded-full bg-warning" />
                Ambiente demonstrativo
              </button>
            )}
            <button
              aria-label={theme === 'dark' ? 'Ativar tema claro' : 'Ativar tema escuro'}
              className="grid h-[39px] w-[39px] place-items-center rounded-[11px] border border-border bg-surface shadow-sm hover:border-border-strong"
              onClick={toggleTheme}
              type="button"
            >
              {theme === 'dark' ? <Sun size={19} /> : <Moon size={19} />}
            </button>
          </div>
        </header>}

        <section className="mx-auto w-full max-w-[1320px] overflow-x-hidden px-[clamp(16px,4vw,56px)] pb-14 pt-8">
          {/* Stays mounted so an intake in progress survives navigation. */}
          <NewAnalysisPage
            visible={section.id === 'intake'}
            cases={caseList.cases}
            onCaseConfirmed={(newCase) => {
              caseList.add(newCase)
              route.goTo('reports')
            }}
            onOpenCase={route.openReport}
          />
          {section.id === 'overview' && (
            <section className="flex items-center justify-between gap-5 rounded-2xl border border-border bg-surface p-6 shadow-sm max-[580px]:flex-col max-[580px]:items-stretch">
              <div>
                <Eyebrow>RESUMO OPERACIONAL</Eyebrow>
                <h2 className="mb-0 mt-1 font-display text-xl">Fluxo demonstrativo pronto para revisão</h2>
              </div>
              <PrimaryButton onClick={() => route.goTo('intake')}>Nova análise</PrimaryButton>
            </section>
          )}
          {section.id === 'reports' && (
            <ReportsPage
              cases={caseList.cases}
              reportId={reportId}
              onBack={() => route.goTo('reports')}
              onDeleted={(caseId) => {
                caseList.remove(caseId)
                route.goTo('reports')
              }}
              onOpen={route.openReport}
            />
          )}
          {section.id === 'saved-exams' && <SavedExamsPage cases={caseList.cases} />}
          {section.id === 'protocol' && <ProtocolPage />}
          {section.id === 'settings' && (
            <section className="mt-5 rounded-2xl border border-border bg-surface p-6 shadow-sm">
              <Eyebrow>SISTEMA</Eyebrow>
              <h2 className="mb-0 mt-1 font-display text-xl">Configurações</h2>
              <p className="mb-0 mt-2 text-sm text-text-secondary">As configurações estarão disponíveis em uma próxima etapa.</p>
            </section>
          )}
          {section.id === 'benchmark' && <BenchmarkOcrPage />}
          {section.id === 'roadmap' && <RoadmapPage />}
        </section>
      </main>
    </div>
  )
}

