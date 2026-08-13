import { useState } from 'react'
import clsx from 'clsx'
import {
  Check,
  ChevronDown,
  Circle,
  Database,
  Flag,
  ShieldCheck,
  UserCheck,
  Users,
} from 'lucide-react'

type RoadmapItem = {
  label: string
  completed: boolean
}

type RoadmapPhase = {
  id: string
  number: number
  title: string
  description: string
  status: string
  badge: 'Estamos aqui' | 'Depois do aval' | 'Entrega do MVP'
  gate: string
  items: RoadmapItem[]
}

const phases: RoadmapPhase[] = [
  {
    id: 'aprovacao-tiago',
    number: 1,
    title: 'Aprovar a experiência',
    description:
      'Validar com o Dr. Tiago toda a experiência do sistema usando dados falsos e sem backend.',
    status: 'Aguardando aprovação do Dr. Tiago',
    badge: 'Estamos aqui',
    gate:
      'O Dr. Tiago aprova esta experiência como base para a construção do sistema real.',
    items: [
      {
        label: 'Estrutura principal da plataforma',
        completed: true,
      },
      {
        label: 'Envio simulado dos exames',
        completed: true,
      },
      {
        label: 'Conferência dos documentos recebidos',
        completed: true,
      },
      {
        label: 'Visualização dos dados extraídos',
        completed: true,
      },
      {
        label: 'Recomendação e relatório preliminar',
        completed: true,
      },
      {
        label: 'Ajustes de clareza e facilidade de uso',
        completed: false,
      },
      {
        label: 'Revisão completa da experiência com o Dr. Tiago',
        completed: false,
      },
      {
        label: 'Aval final do Dr. Tiago',
        completed: false,
      },
    ],
  },
  {
    id: 'sistema-real',
    number: 2,
    title: 'Construir o sistema real',
    description:
      'Manter a experiência aprovada e substituir, aos poucos, as simulações por dados e funcionamento reais.',
    status: 'Bloqueada até o aval do Dr. Tiago',
    badge: 'Depois do aval',
    gate:
      'Um caso real percorre todo o fluxo, desde o envio dos exames até a revisão médica.',
    items: [
      {
        label: 'Backend inicial do sistema',
        completed: false,
      },
      {
        label: 'Envio real de exames',
        completed: false,
      },
      {
        label: 'Armazenamento dos arquivos',
        completed: false,
      },
      {
        label: 'Leitura inicial das informações dos exames',
        completed: false,
      },
      {
        label: 'Dados reais no lugar dos dados simulados',
        completed: false,
      },
      {
        label: 'Primeiras regras clínicas automatizadas',
        completed: false,
      },
      {
        label: 'Geração e revisão de um relatório real',
        completed: false,
      },
    ],
  },
  {
    id: 'primeira-versao',
    number: 3,
    title: 'Liberar a primeira versão para uso',
    description:
      'Deixar o fluxo principal estável e simples o suficiente para ser usado por outras pessoas.',
    status: 'Etapa final da primeira versão',
    badge: 'Entrega do MVP',
    gate:
      'Uma pessoa consegue usar o RefratIA sem precisar do Mateus ao lado explicando cada passo.',
    items: [
      {
        label: 'Acesso básico ao sistema',
        completed: false,
      },
      {
        label: 'Tratamento claro de erros e dados ausentes',
        completed: false,
      },
      {
        label: 'Histórico básico dos casos',
        completed: false,
      },
      {
        label: 'Fluxo principal estável',
        completed: false,
      },
      {
        label: 'Validação com os primeiros usuários',
        completed: false,
      },
    ],
  },
]

function getProgress(phase: RoadmapPhase) {
  const completed = phase.items.filter((item) => item.completed).length
  const total = phase.items.length

  return {
    completed,
    pending: total - completed,
    percentage: total === 0
      ? 0
      : Math.round((completed / total) * 100),
  }
}

function PhaseBadge({
  badge,
}: {
  badge: RoadmapPhase['badge']
}) {
  return (
    <span
      className={clsx(
        'inline-flex items-center rounded-full px-3 py-1.5 text-xs font-bold',
        badge === 'Estamos aqui' && 'bg-warning-soft text-warning',
        badge === 'Depois do aval' && 'bg-primary-soft text-primary',
        badge === 'Entrega do MVP' && 'bg-success-soft text-success',
      )}
    >
      {badge}
    </span>
  )
}

function ProgressBar({
  phase,
}: {
  phase: RoadmapPhase
}) {
  const progress = getProgress(phase)
  const current = phase.id === 'aprovacao-tiago'

  return (
    <div>
      <div className="mb-2.5 flex items-center justify-between gap-4 text-sm">
        <span className="font-semibold text-text-secondary">
          {progress.completed} de {phase.items.length} concluídos
        </span>

        <strong
          className={clsx(
            'font-display',
            current ? 'text-warning' : 'text-text-primary',
          )}
        >
          {progress.percentage}%
        </strong>
      </div>

      <div
        aria-label={`Progresso de ${phase.title}: ${progress.percentage}%`}
        aria-valuemax={100}
        aria-valuemin={0}
        aria-valuenow={progress.percentage}
        className="h-2.5 overflow-hidden rounded-full bg-border"
        role="progressbar"
      >
        <div
          className={clsx(
            'h-full rounded-full transition-[width] duration-500',
            current ? 'bg-warning' : 'bg-primary',
          )}
          style={{ width: `${progress.percentage}%` }}
        />
      </div>
    </div>
  )
}

function PhaseIcon({
  phase,
}: {
  phase: RoadmapPhase
}) {
  const className = 'h-6 w-6'

  if (phase.id === 'aprovacao-tiago') {
    return <UserCheck className={className} />
  }

  if (phase.id === 'sistema-real') {
    return <Database className={className} />
  }

  return <Users className={className} />
}

function PhaseCard({
  phase,
  expanded,
  onToggle,
}: {
  phase: RoadmapPhase
  expanded: boolean
  onToggle: () => void
}) {
  const current = phase.id === 'aprovacao-tiago'
  const progress = getProgress(phase)
  const detailsId = `roadmap-phase-${phase.id}`

  return (
    <article
      className={clsx(
        'relative overflow-hidden rounded-2xl border bg-surface shadow-sm',
        current
          ? 'border-warning'
          : 'border-border',
      )}
    >
      {current && (
        <div className="absolute inset-y-0 left-0 w-1.5 bg-warning" />
      )}

      <button
        aria-controls={detailsId}
        aria-expanded={expanded}
        className="flex w-full items-start gap-4 border-0 bg-transparent p-6 text-left max-[580px]:p-5"
        onClick={onToggle}
        type="button"
      >
        <span
          className={clsx(
            'grid h-12 w-12 flex-none place-items-center rounded-xl',
            current
              ? 'bg-warning-soft text-warning'
              : 'bg-primary-soft text-primary',
          )}
        >
          <PhaseIcon phase={phase} />
        </span>

        <span className="min-w-0 flex-1">
          <span className="flex flex-wrap items-center justify-between gap-3">
            <span>
              <small className="block text-xs font-bold tracking-[0.12em] text-text-muted">
                ETAPA {phase.number}
              </small>

              <strong className="mt-1 block font-display text-xl tracking-[-0.03em]">
                {phase.title}
              </strong>
            </span>

            <PhaseBadge badge={phase.badge} />
          </span>

          <span className="mt-3 block max-w-[820px] text-sm leading-7 text-text-secondary">
            {phase.description}
          </span>

          <span
            className={clsx(
              'mt-3 block text-sm font-semibold',
              current ? 'text-warning' : 'text-text-secondary',
            )}
          >
            {phase.status}
          </span>
        </span>

        <ChevronDown
          aria-hidden="true"
          className={clsx(
            'mt-2 flex-none text-primary transition-transform duration-300',
            expanded && 'rotate-180',
          )}
          size={21}
        />
      </button>

      <div className="px-6 pb-6 max-[580px]:px-5 max-[580px]:pb-5">
        <ProgressBar phase={phase} />

        <div
          aria-hidden={!expanded}
          className={clsx(
            'grid transition-[grid-template-rows,opacity] duration-300',
            expanded
              ? 'grid-rows-[1fr] opacity-100'
              : 'grid-rows-[0fr] opacity-0',
          )}
          id={detailsId}
        >
          <div className="overflow-hidden">
            <div className="mt-6 border-t border-border pt-6">
              <ul className="m-0 grid list-none grid-cols-2 gap-3 p-0 max-[820px]:grid-cols-1">
                {phase.items.map((item) => (
                  <li
                    className={clsx(
                      'flex min-h-14 items-start gap-3 rounded-xl border px-4 py-3.5 text-sm leading-6',
                      item.completed
                        ? 'border-success/30 bg-success-soft text-text-primary'
                        : 'border-border bg-surface-muted text-text-secondary',
                    )}
                    key={item.label}
                  >
                    {item.completed ? (
                      <Check
                        aria-hidden="true"
                        className="mt-1 flex-none text-success"
                        size={18}
                        strokeWidth={3}
                      />
                    ) : (
                      <Circle
                        aria-hidden="true"
                        className="mt-1 flex-none text-text-muted"
                        size={18}
                      />
                    )}

                    <span>
                      {item.label}

                      <span className="sr-only">
                        {' '}
                        — {item.completed ? 'concluído' : 'pendente'}
                      </span>
                    </span>
                  </li>
                ))}
              </ul>

              <div
                className={clsx(
                  'mt-5 rounded-xl border p-4',
                  current
                    ? 'border-warning/40 bg-warning-soft'
                    : 'border-primary-border bg-primary-soft',
                )}
              >
                <span className="text-xs font-bold tracking-[0.12em] text-text-muted">
                  CRITÉRIO PARA AVANÇAR
                </span>

                <p className="mb-0 mt-2 text-sm leading-7 text-text-secondary">
                  {phase.gate}
                </p>
              </div>

              {progress.pending > 0 && (
                <p className="mb-0 mt-4 text-sm text-text-muted">
                  {progress.pending} passos ainda estão pendentes nesta etapa.
                </p>
              )}
            </div>
          </div>
        </div>
      </div>
    </article>
  )
}

export default function RoadmapPage() {
  const [expandedPhase, setExpandedPhase] = useState<string | null>(
    phases[0].id,
  )

  const currentPhase = phases[0]
  const currentProgress = getProgress(currentPhase)

  return (
    <div className="min-w-0">
      <section className="hero-bg rounded-2xl border border-primary-border p-7 max-[580px]:p-5">
        <div className="grid grid-cols-[minmax(0,1.25fr)_minmax(280px,.75fr)] items-center gap-8 max-[900px]:grid-cols-1">
          <div>
            <span className="text-xs font-bold tracking-[0.13em] text-primary">
              CAMINHO ATÉ A PRIMEIRA VERSÃO
            </span>

            <h2 className="mb-0 mt-3 font-display text-[clamp(28px,3vw,40px)] leading-[1.12] tracking-[-0.045em]">
              Roadmap RefratIA
            </h2>

            <p className="mb-0 mt-4 max-w-[760px] text-sm leading-7 text-text-secondary">
              Primeiro validamos toda a experiência com dados falsos. Somente
              depois da aprovação do Dr. Tiago começamos a construir o sistema
              real.
            </p>
          </div>

          <div className="rounded-2xl border border-warning/40 bg-warning-soft p-5">
            <ShieldCheck
              aria-hidden="true"
              className="text-warning"
              size={24}
            />

            <span className="mt-4 block text-xs font-bold tracking-[0.12em] text-warning">
              REGRA PARA COMEÇAR O BACKEND
            </span>

            <p className="mb-0 mt-2 font-display text-xl leading-snug tracking-[-0.03em]">
              O backend só começa depois da aprovação do Dr. Tiago.
            </p>
          </div>
        </div>
      </section>

      <section
        aria-labelledby="roadmap-current"
        className="mt-6 rounded-2xl border border-warning bg-surface p-6 shadow-sm"
      >
        <div className="grid grid-cols-[minmax(0,1.2fr)_minmax(280px,.8fr)] items-center gap-7 max-[900px]:grid-cols-1">
          <div>
            <div className="flex items-center gap-4">
              <span className="grid h-11 w-11 flex-none place-items-center rounded-xl bg-warning-soft text-warning">
                <Flag size={21} />
              </span>

              <div>
                <span className="text-xs font-bold tracking-[0.12em] text-warning">
                  ESTAMOS AQUI
                </span>

                <h2
                  className="mb-0 mt-1 font-display text-xl tracking-[-0.03em]"
                  id="roadmap-current"
                >
                  Aprovação da experiência
                </h2>
              </div>
            </div>

            <p className="mb-0 mt-4 max-w-[740px] text-sm leading-7 text-text-secondary">
              Os dados continuam falsos e não existe backend. Isso é
              intencional: precisamos ajustar a experiência rapidamente até
              que o Dr. Tiago esteja satisfeito com o fluxo completo.
            </p>

            <div className="mt-5 flex flex-wrap gap-2">
              {[
                'Dados falsos',
                'Sem backend',
                'Fluxo completo simulado',
              ].map((label) => (
                <span
                  className="rounded-full border border-border bg-surface-muted px-3 py-1.5 text-xs font-semibold text-text-secondary"
                  key={label}
                >
                  {label}
                </span>
              ))}
            </div>
          </div>

          <div className="rounded-2xl border border-border bg-surface-muted p-5">
            <span className="text-xs font-bold tracking-[0.12em] text-text-muted">
              DECISÃO NECESSÁRIA
            </span>

            <blockquote className="mb-0 mt-3 border-l-2 border-warning pl-4 text-sm leading-7 text-text-secondary">
              “Mateus, se esses dados não fossem falsos, eu estaria satisfeito
              com o sistema.”
            </blockquote>

            <div className="mt-5 rounded-xl bg-warning-soft px-4 py-3 text-sm font-bold text-warning">
              Aguardando aprovação do Dr. Tiago
            </div>

            <div className="mt-5">
              <ProgressBar phase={currentPhase} />
            </div>

            <p className="mb-0 mt-3 text-xs text-text-muted">
              {currentProgress.pending} passos ainda precisam ser validados.
            </p>
          </div>
        </div>
      </section>

      <section
        aria-labelledby="roadmap-phases"
        className="mt-10"
      >
        <span className="text-xs font-bold tracking-[0.13em] text-primary">
          TRÊS ETAPAS
        </span>

        <h2
          className="mb-0 mt-2 font-display text-2xl tracking-[-0.035em]"
          id="roadmap-phases"
        >
          Da simulação à primeira versão utilizável
        </h2>

        <p className="mb-0 mt-3 max-w-[760px] text-sm leading-7 text-text-secondary">
          O roadmap termina quando o sistema estiver minimamente pronto para
          ser usado por outras pessoas.
        </p>

        <div className="mt-6 grid gap-4">
          {phases.map((phase) => (
            <PhaseCard
              expanded={expandedPhase === phase.id}
              key={phase.id}
              onToggle={() => {
                setExpandedPhase((current) =>
                  current === phase.id
                    ? null
                    : phase.id,
                )
              }}
              phase={phase}
            />
          ))}
        </div>
      </section>
    </div>
  )
}
