interface ExamProtocolChecklistProps {
  files: File[]
}

type Eye = 'OD' | 'OS' | 'AO'

interface ParsedExamFile {
  type: string
  eye: Eye
}

interface ExamDefinition {
  key: string
  label: string
}

const requiredExams: ExamDefinition[] = [
  {
    key: 'PENTACAM',
    label: 'Pentacam',
  },
  {
    key: 'EYESUITE',
    label: 'Biometria óptica — EyeSuite',
  },
  {
    key: 'MICROSCOPIA_ESPECULAR',
    label: 'Microscopia especular',
  },
]

const optionalExams: ExamDefinition[] = [
  {
    key: 'RETINA',
    label: 'Retinografia',
  },
]

function parseFilename(
  name: string,
): ParsedExamFile | null {
  const stem = name.replace(/\.[^.]+$/, '')
  const parts = stem.split('__')

  if (parts.length < 2) return null

  const type = parts[0].trim().toUpperCase()
  const eye = parts[1].trim().toUpperCase()

  if (
    eye !== 'OD' &&
    eye !== 'OS' &&
    eye !== 'AO'
  ) {
    return null
  }

  return {
    type,
    eye,
  }
}

function getExamStatus(
  parsed: ParsedExamFile[],
  type: string,
) {
  const eyes = new Set(
    parsed
      .filter((file) => file.type === type)
      .map((file) => file.eye),
  )

  const hasAO = eyes.has('AO')
  const hasOD = eyes.has('OD')
  const hasOS = eyes.has('OS')

  const received =
    hasAO ||
    hasOD ||
    hasOS

  const complete =
    hasAO ||
    (hasOD && hasOS)

  const partial =
    received && !complete

  let detail = 'Não enviado'

  if (hasAO) {
    detail = 'AO recebido'
  } else if (hasOD && hasOS) {
    detail = 'OD + OS recebidos'
  } else if (hasOD) {
    detail = 'OD recebido · falta OS'
  } else if (hasOS) {
    detail = 'OS recebido · falta OD'
  }

  return {
    received,
    complete,
    partial,
    detail,
  }
}

function ExamRow({
  exam,
  parsed,
  optional = false,
}: {
  exam: ExamDefinition
  parsed: ParsedExamFile[]
  optional?: boolean
}) {
  const status = getExamStatus(
    parsed,
    exam.key,
  )

  return (
    <div className="flex items-center gap-3 rounded-xl border border-border bg-surface px-3 py-3">
      <span
        className={[
          'grid h-7 w-7 flex-none place-items-center rounded-full text-sm font-bold',
          status.complete
            ? 'bg-success-soft text-success'
            : status.partial
              ? 'bg-warning-soft text-warning'
              : 'bg-surface-muted text-text-muted',
        ].join(' ')}
      >
        {status.complete
          ? '✓'
          : status.partial
            ? '◐'
            : '○'}
      </span>

      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2">
          <strong className="text-sm text-text-primary">
            {exam.label}
          </strong>

          {optional && (
            <span className="rounded-full bg-surface-muted px-2 py-0.5 text-[10px] font-bold uppercase tracking-[0.08em] text-text-muted">
              Opcional
            </span>
          )}
        </div>

        <span
          className={[
            'mt-0.5 block text-xs',
            status.partial
              ? 'text-warning'
              : 'text-text-muted',
          ].join(' ')}
        >
          {status.detail}
        </span>
      </div>
    </div>
  )
}

export default function ExamProtocolChecklist({
  files,
}: ExamProtocolChecklistProps) {
  const parsed = files
    .map((file) => parseFilename(file.name))
    .filter(
      (file): file is ParsedExamFile =>
        file !== null,
    )

  const requiredStatuses =
    requiredExams.map((exam) => ({
      exam,
      ...getExamStatus(parsed, exam.key),
    }))

  const received =
    requiredStatuses.filter(
      (item) => item.received,
    ).length

  const missing =
    requiredExams.length - received

  return (
    <div className="mt-4 rounded-xl border border-border bg-surface-muted p-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <span className="text-xs font-bold uppercase tracking-[0.12em] text-primary">
            Exames obrigatórios
          </span>

          <div className="mt-1 flex items-baseline gap-2">
            <strong className="font-display text-xl text-text-primary">
              {received}/{requiredExams.length}
            </strong>

            <span className="text-xs text-text-muted">
              recebidos
            </span>
          </div>
        </div>

        <span className="rounded-full border border-border bg-surface px-3 py-1 text-xs font-semibold text-text-secondary">
          {missing === 0
            ? 'Obrigatórios recebidos'
            : `${missing} ${
                missing === 1
                  ? 'obrigatório faltando'
                  : 'obrigatórios faltando'
              }`}
        </span>
      </div>

      <div className="mt-4 h-2 overflow-hidden rounded-full bg-surface">
        <div
          className="h-full rounded-full bg-primary transition-[width] duration-300"
          style={{
            width: `${Math.round(
              (received /
                requiredExams.length) *
                100,
            )}%`,
          }}
        />
      </div>

      <div className="mt-4 grid gap-2 sm:grid-cols-2">
        {requiredExams.map((exam) => (
          <ExamRow
            exam={exam}
            key={exam.key}
            parsed={parsed}
          />
        ))}
      </div>

      <div className="my-5 border-t border-border" />

      <div>
        <span className="text-xs font-bold uppercase tracking-[0.12em] text-text-muted">
          Exames opcionais
        </span>

        <div className="mt-3 grid gap-2 sm:grid-cols-2">
          {optionalExams.map((exam) => (
            <ExamRow
              exam={exam}
              key={exam.key}
              optional
              parsed={parsed}
            />
          ))}
        </div>
      </div>

      <p className="mb-0 mt-4 text-[11px] leading-5 text-text-muted">
        Pentacam, EyeSuite e microscopia especular compõem o conjunto obrigatório.
        A retinografia pode acompanhar o caso como exame opcional e não altera a contagem principal.
      </p>
    </div>
  )
}
