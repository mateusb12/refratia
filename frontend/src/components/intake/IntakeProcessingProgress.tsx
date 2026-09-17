import IntakeFileProgressItem from './IntakeFileProgressItem'

import type {
  IntakeFileProgressState,
} from './intakeProgress'

interface IntakeProcessingProgressProps {
  files: File[]
  fileProgress: Record<
    string,
    IntakeFileProgressState
  >
  overallProgress: number
  overallMessage: string
  overallStage: string
  overallFilename: string
  elapsedSeconds: number
}

export default function IntakeProcessingProgress({
  files,
  fileProgress,
  overallProgress,
  overallMessage,
  overallStage,
  overallFilename,
  elapsedSeconds,
}: IntakeProcessingProgressProps) {
  const elapsedLabel =
    `${String(
      Math.floor(elapsedSeconds / 60),
    ).padStart(2, '0')}:${String(
      elapsedSeconds % 60,
    ).padStart(2, '0')}`

  const finished =
    files.filter((file) => {
      const status =
        fileProgress[file.name]?.status

      return (
        status === 'extracted' ||
        status === 'identified' ||
        status === 'failed'
      )
    }).length

  return (
    <div className="mt-4 rounded-xl border border-primary-border bg-primary-soft p-4">
      <div
        aria-label={`Progresso geral da análise: ${overallProgress}%`}
        aria-live="polite"
      >
        <div className="flex items-center justify-between gap-3 text-xs font-bold">
          <span className="text-primary">
            {overallMessage ||
              'Processando documentos'}
          </span>

          <span className="text-text-secondary">
            {overallProgress}%
          </span>
        </div>

        <div
          aria-valuemax={100}
          aria-valuemin={0}
          aria-valuenow={overallProgress}
          className="mt-2 h-2 overflow-hidden rounded-full bg-surface"
          role="progressbar"
        >
          <div
            className="h-full rounded-full bg-primary transition-[width] duration-500"
            style={{
              width: `${overallProgress}%`,
            }}
          />
        </div>

        <div className="mt-2 flex items-center gap-2 text-xs text-text-secondary">
          <span className="inline-block h-2 w-2 animate-pulse rounded-full bg-primary" />

          <span className="truncate">
            {overallFilename ||
              overallStage ||
              'OCR local'}
          </span>

          <span className="ml-auto flex-none font-mono text-text-muted">
            {elapsedLabel}
          </span>
        </div>

        <p className="mb-0 mt-2 text-[11px] text-text-muted">
          Progresso geral informado pelo backend em tempo real.
        </p>
      </div>

      <div className="my-4 border-t border-primary-border/60" />

      <div className="mb-4 flex items-center justify-between gap-3">
        <div>
          <strong className="block text-xs text-text-primary">
            Processamento por arquivo
          </strong>

          <span className="mt-1 block text-[11px] text-text-muted">
            Cada exame avança pelas etapas reais do parser.
          </span>
        </div>

        <span className="rounded-full border border-border bg-surface px-3 py-1 text-[10px] font-bold text-text-secondary">
          {finished}/{files.length} finalizados
        </span>
      </div>

      <div>
        {files.map((file, index) => (
          <IntakeFileProgressItem
            file={file}
            key={`${file.name}-${file.size}-${file.lastModified}`}
            last={index === files.length - 1}
            progress={fileProgress[file.name]}
          />
        ))}
      </div>
    </div>
  )
}
