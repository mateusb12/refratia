import { classifyFilename } from './filenameClassification'

interface ExamProtocolChecklistProps {
  files: File[]
}

type Eye = 'OD' | 'OS' | 'AO'

interface ParsedExamFile {
  type: string
  eye: Eye
  file: File
}

interface ExamDefinition {
  key: string
  label: string
}

interface RequiredExamSlot extends ExamDefinition {
  eye: Eye
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

const requiredExamSlots: RequiredExamSlot[] = requiredExams.flatMap((exam) => (
  (['OD', 'OS'] as const).map((eye) => ({ ...exam, eye }))
))

function getDisplayedRequiredSlots(parsed: ParsedExamFile[]): RequiredExamSlot[] {
  return requiredExams.flatMap((exam) => {
    const hasAO = parsed.some((file) => file.type === exam.key && file.eye === 'AO')
    return hasAO
      ? [{ ...exam, eye: 'AO' as Eye }]
      : (['OD', 'OS'] as const).map((eye): RequiredExamSlot => ({ ...exam, eye }))
  })
}

const optionalExams: ExamDefinition[] = [
  {
    key: 'RETINA',
    label: 'Retinografia',
  },
]

function parseFilename(
  name: string,
): Omit<ParsedExamFile, 'file'> | null {
  const classification = classifyFilename(name)
  if (!classification) return null

  const typeByExam: Record<string, string> = {
    pentacam_corneal_tomography: 'PENTACAM',
    iol_calculation: 'EYESUITE',
    refractometry: 'AUTO',
    specular_microscopy: 'MICROSCOPIA_ESPECULAR',
    fundus_retinography: 'RETINA',
    oct_retina: 'OCT',
  }

  return { type: typeByExam[classification.examType], eye: classification.eye }
}

function fileDetails(file: File) {
  const extension = file.name.split('.').pop()?.toUpperCase() ?? 'ARQUIVO'
  const size = file.size >= 1024 * 1024
    ? `${(file.size / (1024 * 1024)).toFixed(1)} MB`
    : `${Math.round(file.size / 1024)} KB`
  return `${extension} · ${size}`
}

function fileIcon(file: File) {
  const extension = file.name.split('.').pop()?.toLowerCase()
  const icon = extension === 'pdf' ? 'pdf.png' : extension === 'png' ? 'png.png' : 'jpeg.png'
  return `${import.meta.env.BASE_URL}${icon}`
}

function eyeLabel(eye: Eye) {
  if (eye === 'AO') return 'Ambos os olhos'
  if (eye === 'OD') return 'Olho direito'
  return 'Olho esquerdo'
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

function getSlotFiles(
  parsed: ParsedExamFile[],
  slot: RequiredExamSlot,
) {
  return parsed.filter(
    (file) => file.type === slot.key && (file.eye === slot.eye || file.eye === 'AO'),
  )
}

function RequiredExamSlotRow({
  slot,
  parsed,
}: {
  slot: RequiredExamSlot
  parsed: ParsedExamFile[]
}) {
  const matchingFiles = getSlotFiles(parsed, slot)
  const received = matchingFiles.length > 0

  return (
    <div className="flex min-w-0 items-center gap-3 rounded-xl border border-border bg-surface px-3 py-2.5">
      {received ? (
        <img
          alt=""
          className="h-9 w-9 flex-none object-contain"
          src={fileIcon(matchingFiles[0].file)}
        />
      ) : (
        <span className="grid h-7 w-7 flex-none place-items-center rounded-full bg-surface-muted text-sm font-bold text-text-muted">
          ○
        </span>
      )}

      <div className="min-w-0">
        <strong className="block truncate text-sm text-text-primary">
          {slot.label} · {slot.eye}
        </strong>
        <span className={[
          'mt-0.5 block text-xs',
          received ? 'text-success' : 'text-text-muted',
        ].join(' ')}>
          {received ? eyeLabel(slot.eye) : 'Falta arquivo'}
        </span>
        {matchingFiles.map(({ file }) => (
          <span
            className="mt-0.5 block max-w-full truncate text-[11px] text-text-muted"
            key={`${file.name}-${file.lastModified}`}
            title={`${file.name} · ${fileDetails(file)}`}
          >
            {file.name} · {fileDetails(file)}
          </span>
        ))}
      </div>
    </div>
  )
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
  const matchingFiles = parsed.filter((file) => file.type === exam.key)

  return (
    <div className="flex min-w-0 items-center gap-3 rounded-xl border border-border bg-surface px-3 py-2.5">
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
        {matchingFiles.map(({ file }) => (
          <span
            className="mt-0.5 block max-w-full truncate text-[11px] text-text-muted"
            key={`${file.name}-${file.lastModified}`}
            title={`${file.name} · ${fileDetails(file)}`}
          >
            {file.name} · {fileDetails(file)}
          </span>
        ))}
      </div>
    </div>
  )
}

export default function ExamProtocolChecklist({
  files,
}: ExamProtocolChecklistProps) {
  const parsed = files
    .map((file) => {
      const parsedFile = parseFilename(file.name)
      return parsedFile ? { ...parsedFile, file } : null
    })
    .filter(
      (file): file is ParsedExamFile =>
        file !== null,
    )

  const requiredStatuses = requiredExamSlots.map((slot) => ({
    slot,
    received: getSlotFiles(parsed, slot).length > 0,
  }))

  const received =
    requiredStatuses.filter(
      (item) => item.received,
    ).length

  const missing =
    requiredExamSlots.length - received
  const displayedRequiredSlots = getDisplayedRequiredSlots(parsed)

  return (
    <div className="mt-4 rounded-xl border border-border bg-surface-muted p-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <span className="block text-[10px] font-bold uppercase tracking-[0.12em] text-primary">
            Etapa 2 · Conferir documentos
          </span>
          <span className="text-xs font-bold uppercase tracking-[0.12em] text-primary">
            Exames obrigatórios
          </span>

          <div className="mt-1 flex items-baseline gap-2">
            <strong className="font-display text-xl text-text-primary">
              {received}/{requiredExamSlots.length}
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
                requiredExamSlots.length) *
                100,
            )}%`,
          }}
        />
      </div>

      <div className="mt-4 grid gap-2 sm:grid-cols-2">
        {displayedRequiredSlots.map((slot) => (
          <RequiredExamSlotRow
            key={`${slot.key}-${slot.eye}`}
            parsed={parsed}
            slot={slot}
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
        Cada exame obrigatório é conferido separadamente por olho. A retinografia pode acompanhar o caso como exame opcional e não altera a contagem principal.
      </p>
    </div>
  )
}
