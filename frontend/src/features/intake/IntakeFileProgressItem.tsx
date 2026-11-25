import {
  AlertTriangle,
  Check,
  Circle,
  Clock3,
  LoaderCircle,
} from 'lucide-react'

import type {
  IntakeFileProgressState,
  IntakeFileProcessingStatus,
} from './intakeProgress'

interface IntakeFileProgressItemProps {
  file: File
  progress?: IntakeFileProgressState
  last: boolean
}

function formatProcessingDuration(
  startedAtMs?: number,
  finishedAtMs?: number,
) {
  if (!startedAtMs) {
    return ''
  }

  const endTimeMs =
    finishedAtMs ??
    Date.now()

  const totalSeconds =
    Math.max(
      0,
      Math.floor(
        (endTimeMs - startedAtMs) /
          1000,
      ),
    )

  const minutes =
    Math.floor(totalSeconds / 60)

  const seconds =
    totalSeconds % 60

  if (minutes === 0) {
    return `${seconds}s`
  }

  return `${minutes}m${String(seconds).padStart(2, '0')}s`
}

function statusLabel(
  status: IntakeFileProcessingStatus,
) {
  switch (status) {
    case 'processing':
      return 'Processando'
    case 'extracted':
      return 'Extraído'
    case 'identified':
      return 'Identificado'
    case 'failed':
      return 'Falhou'
    default:
      return 'Aguardando'
  }
}

function statusClasses(
  status: IntakeFileProcessingStatus,
) {
  switch (status) {
    case 'processing':
      return {
        circle:
          'border-primary bg-primary-soft text-primary',
        bar: 'bg-primary',
        badge:
          'border-primary-border bg-primary-soft text-primary',
        line: 'bg-primary/40',
      }

    case 'extracted':
      return {
        circle:
          'border-success/40 bg-success-soft text-success',
        bar: 'bg-success',
        badge:
          'border-success/30 bg-success-soft text-success',
        line: 'bg-success/40',
      }

    case 'identified':
      return {
        circle:
          'border-warning/40 bg-surface text-warning',
        bar: 'bg-warning',
        badge:
          'border-warning/30 bg-surface text-warning',
        line: 'bg-warning/40',
      }

    case 'failed':
      return {
        circle:
          'border-danger/40 bg-danger-soft text-danger',
        bar: 'bg-danger',
        badge:
          'border-danger/30 bg-danger-soft text-danger',
        line: 'bg-danger/40',
      }

    default:
      return {
        circle:
          'border-border bg-surface text-text-muted',
        bar: 'bg-border-strong',
        badge:
          'border-border bg-surface text-text-muted',
        line: 'bg-border',
      }
  }
}

function StatusIcon({
  status,
}: {
  status: IntakeFileProcessingStatus
}) {
  switch (status) {
    case 'processing':
      return (
        <LoaderCircle
          className="animate-spin"
          size={16}
        />
      )

    case 'extracted':
    case 'identified':
      return (
        <Check
          size={16}
          strokeWidth={3}
        />
      )

    case 'failed':
      return (
        <AlertTriangle
          size={15}
        />
      )

    default:
      return (
        <Circle
          size={11}
        />
      )
  }
}

export default function IntakeFileProgressItem({
  file,
  progress,
  last,
}: IntakeFileProgressItemProps) {
  const current =
    progress ?? {
      percent: 0,
      status: 'waiting' as const,
      stage: '',
      message: 'Aguardando processamento',
    }

  const classes =
    statusClasses(current.status)

  const durationLabel =
    formatProcessingDuration(
      current.startedAtMs,
      current.finishedAtMs,
    )

  const metadata = [
    current.examType,
    current.eye,
  ]
    .filter(Boolean)
    .join(' · ')

  return (
    <div className="relative flex gap-3">
      <div className="relative flex w-8 flex-none justify-center">
        {!last && (
          <span
            aria-hidden="true"
            className={`absolute bottom-0 top-8 w-px ${classes.line}`}
          />
        )}

        <span
          aria-current={
            current.status === 'processing'
              ? 'step'
              : undefined
          }
          className={[
            'relative z-10 grid h-8 w-8 place-items-center rounded-full border transition-colors',
            classes.circle,
          ].join(' ')}
        >
          <StatusIcon
            status={current.status}
          />
        </span>
      </div>

      <div
        className={[
          'mb-4 min-w-0 flex-1 rounded-xl border p-3.5',
          current.status === 'processing'
            ? 'border-primary-border bg-primary-soft/40'
            : 'border-border bg-surface',
        ].join(' ')}
      >
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <strong
              className="block truncate text-sm text-text-primary"
              title={file.name}
            >
              {file.name}
            </strong>

            <span className="mt-1 block text-[11px] text-text-muted">
              {metadata ||
                `${(file.size / 1024 / 1024).toFixed(2)} MB`}
            </span>
          </div>

          <div className="flex flex-none items-center gap-2">
            {durationLabel && (
              <span
                className="flex items-center gap-1 font-mono text-[10px] font-semibold text-text-muted"
                title="Tempo de processamento deste exame"
              >
                <Clock3
                  aria-hidden="true"
                  size={12}
                />
                {durationLabel}
              </span>
            )}

            <span
              className={[
                'flex-none rounded-full border px-2.5 py-1 text-[10px] font-bold',
                classes.badge,
              ].join(' ')}
            >
              {statusLabel(current.status)}
            </span>
          </div>
        </div>

        <div className="mt-3 flex items-center justify-between gap-3">
          <span className="min-w-0 truncate text-xs text-text-secondary">
            {current.message}
          </span>

          <strong className="flex-none font-mono text-[11px] text-text-secondary">
            {current.percent}%
          </strong>
        </div>

        <div
          aria-label={`Progresso de ${file.name}: ${current.percent}%`}
          aria-valuemax={100}
          aria-valuemin={0}
          aria-valuenow={current.percent}
          className="mt-2 h-1.5 overflow-hidden rounded-full bg-surface-muted"
          role="progressbar"
        >
          <div
            className={[
              'h-full rounded-full transition-[width] duration-500',
              classes.bar,
            ].join(' ')}
            style={{
              width: `${current.percent}%`,
            }}
          />
        </div>

        {current.stage && (
          <span className="mt-2 block truncate font-mono text-[10px] text-text-muted">
            {current.stage}
          </span>
        )}
      </div>
    </div>
  )
}
