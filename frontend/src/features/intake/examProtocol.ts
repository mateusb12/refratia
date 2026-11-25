import { classifyFilename } from '../../shared/contracts/filenameClassification'

export function hasCompleteOpticalBiometry(files: File[]) {
  const eyes = new Set(
    files
      .map((file) => classifyFilename(file.name))
      .filter((parsed) => parsed?.examType === 'iol_calculation')
      .map((parsed) => parsed!.eye),
  )

  return eyes.has('AO') || (eyes.has('OD') && eyes.has('OS'))
}
