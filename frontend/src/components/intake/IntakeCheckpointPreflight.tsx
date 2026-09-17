import {
  CheckCircle2,
  Circle,
  Database,
  LoaderCircle,
  RefreshCw,
  RotateCcw,
  TriangleAlert,
} from 'lucide-react'

import {
  intakeFileCheckpointKey,
  type IntakeFileCheckpointState,
} from './intakeCheckpoint'

interface IntakeCheckpointPreflightProps {
  files: File[]
  states: Record<
    string,
    IntakeFileCheckpointState
  >
  checking: boolean
  error: string
  forceReprocess: boolean
  disabled?: boolean
  onForceReprocessChange: (
    value: boolean,
  ) => void
}

function statusMetadata(
  status: IntakeFileCheckpointState['status'],
) {
  switch (status) {
    case 'processed':
      return {
        label: 'Já processado',
        detail:
          'Resultado salvo. O OCR será pulado e a análise anterior será reutilizada.',
        circle:
          'border-success/40 bg-success-soft text-success',
        badge:
          'border-success/30 bg-success-soft text-success',
        Icon: CheckCircle2,
      }

    case 'saved':
      return {
        label: 'Salvo · incompleto',
        detail:
          'O arquivo original foi preservado, mas o processamento anterior não terminou. Será processado novamente.',
        circle:
          'border-warning/40 bg-warning-soft text-warning',
        badge:
          'border-warning/30 bg-warning-soft text-warning',
        Icon: RotateCcw,
      }

    default:
      return {
        label: 'Novo',
        detail:
          'Arquivo ainda não visto. Será salvo antes do processamento.',
        circle:
          'border-border bg-surface-muted text-text-muted',
        badge:
          'border-border bg-surface-muted text-text-secondary',
        Icon: Circle,
      }
  }
}

export default function IntakeCheckpointPreflight({
  files,
  states,
  checking,
  error,
  forceReprocess,
  disabled = false,
  onForceReprocessChange,
}: IntakeCheckpointPreflightProps) {
  const values =
    Object.values(states)

  const processedCount =
    values.filter(
      (item) =>
        item.status ===
        'processed',
    ).length

  const savedCount =
    values.filter(
      (item) =>
        item.status ===
        'saved',
    ).length

  const newCount =
    values.filter(
      (item) =>
        item.status ===
        'new',
    ).length

  return (
    <div className="mt-4 overflow-hidden rounded-xl border border-border bg-surface-muted/50">
      <div className="flex flex-wrap items-start justify-between gap-3 border-b border-border px-4 py-3.5">
        <div>
          <div className="flex items-center gap-2">
            <Database
              className="text-primary"
              size={16}
            />

            <strong className="text-sm text-text-primary">
              Verificação de processamento
            </strong>
          </div>

          <p className="mb-0 mt-1 text-[11px] text-text-muted">
            O navegador calcula o SHA-256 e consulta
            resultados anteriores antes da análise.
          </p>
        </div>

        {checking ? (
          <span className="flex items-center gap-1.5 rounded-full border border-primary-border bg-primary-soft px-3 py-1 text-[10px] font-bold text-primary">
            <LoaderCircle
              className="animate-spin"
              size={12}
            />
            Verificando
          </span>
        ) : !error &&
          values.length > 0 ? (
          <div className="flex flex-wrap gap-1.5">
            {processedCount > 0 && (
              <span className="rounded-full border border-success/30 bg-success-soft px-2.5 py-1 text-[10px] font-bold text-success">
                {processedCount} pronto(s)
              </span>
            )}

            {savedCount > 0 && (
              <span className="rounded-full border border-warning/30 bg-warning-soft px-2.5 py-1 text-[10px] font-bold text-warning">
                {savedCount} interrompido(s)
              </span>
            )}

            {newCount > 0 && (
              <span className="rounded-full border border-border bg-surface px-2.5 py-1 text-[10px] font-bold text-text-secondary">
                {newCount} novo(s)
              </span>
            )}
          </div>
        ) : null}
      </div>

      {error && (
        <div className="flex items-start gap-2 border-b border-warning/30 bg-warning-soft px-4 py-3 text-xs text-warning">
          <TriangleAlert
            className="mt-0.5 flex-none"
            size={15}
          />

          <div>
            <strong className="block">
              Não foi possível consultar o histórico.
            </strong>

            <span className="mt-0.5 block text-text-secondary">
              {error} A análise ainda pode ser executada normalmente.
            </span>
          </div>
        </div>
      )}

      <div className="divide-y divide-border">
        {files.map((file) => {
          const state =
            states[
              intakeFileCheckpointKey(
                file,
              )
            ]

          if (!state) {
            return (
              <div
                className="flex items-center gap-3 px-4 py-3"
                key={intakeFileCheckpointKey(
                  file,
                )}
              >
                <span className="grid h-8 w-8 flex-none place-items-center rounded-full border border-border bg-surface text-text-muted">
                  {checking ? (
                    <LoaderCircle
                      className="animate-spin"
                      size={14}
                    />
                  ) : error ? (
                    <TriangleAlert
                      size={14}
                    />
                  ) : (
                    <Circle
                      size={10}
                    />
                  )}
                </span>

                <div className="min-w-0">
                  <strong
                    className="block truncate text-xs text-text-primary"
                    title={file.name}
                  >
                    {file.name}
                  </strong>

                  <span className="mt-0.5 block text-[11px] text-text-muted">
                    {checking
                      ? 'Calculando SHA-256 e consultando storage…'
                      : error
                        ? 'Estado anterior indisponível'
                        : 'Aguardando verificação'}
                  </span>
                </div>
              </div>
            )
          }

          const metadata =
            statusMetadata(
              state.status,
            )

          const StatusIcon =
            metadata.Icon

          const detail =
            state.status ===
              'processed' &&
            forceReprocess
              ? 'Reprocessamento forçado nesta execução. O resultado salvo não será reutilizado.'
              : metadata.detail

          return (
            <div
              className="flex items-start gap-3 px-4 py-3"
              key={intakeFileCheckpointKey(
                file,
              )}
            >
              <span
                className={[
                  'grid h-8 w-8 flex-none place-items-center rounded-full border',
                  metadata.circle,
                ].join(' ')}
              >
                <StatusIcon
                  size={14}
                />
              </span>

              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-start justify-between gap-2">
                  <div className="min-w-0">
                    <strong
                      className="block truncate text-xs text-text-primary"
                      title={file.name}
                    >
                      {file.name}
                    </strong>

                    <span className="mt-0.5 block text-[10px] font-mono text-text-muted">
                      {state.examType ||
                        'EXAME'}
                      {state.eye
                        ? ` · ${state.eye}`
                        : ''}
                      {' · '}
                      {state.sha256.slice(
                        0,
                        10,
                      )}
                      …
                    </span>
                  </div>

                  <span
                    className={[
                      'flex-none rounded-full border px-2.5 py-1 text-[10px] font-bold',
                      metadata.badge,
                    ].join(' ')}
                  >
                    {state.status ===
                      'processed' &&
                    forceReprocess
                      ? 'Reprocessar'
                      : metadata.label}
                  </span>
                </div>

                <p className="mb-0 mt-1.5 text-[11px] leading-relaxed text-text-secondary">
                  {detail}
                </p>
              </div>
            </div>
          )
        })}
      </div>

      {processedCount > 0 &&
        !checking &&
        !error && (
          <label
            className={[
              'flex items-start gap-3 border-t border-border px-4 py-3',
              disabled
                ? 'cursor-not-allowed opacity-60'
                : 'cursor-pointer',
            ].join(' ')}
          >
            <input
              checked={forceReprocess}
              className="mt-0.5 h-4 w-4 accent-current"
              disabled={disabled}
              onChange={(event) =>
                onForceReprocessChange(
                  event.target.checked,
                )
              }
              type="checkbox"
            />

            <span className="min-w-0">
              <strong className="flex items-center gap-1.5 text-xs text-text-primary">
                <RefreshCw
                  size={13}
                />
                Reprocessar arquivos já concluídos
              </strong>

              <span className="mt-0.5 block text-[11px] text-text-muted">
                Força nova execução do OCR para o lote inteiro,
                inclusive arquivos com resultado salvo.
              </span>
            </span>
          </label>
        )}
    </div>
  )
}
