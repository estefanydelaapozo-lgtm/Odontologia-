const surfaceStyle: Record<string, string> = {
  caries: 'caries',
  restauracion: 'restoration',
  endodoncia_realizada: 'endodontic',
  endodoncia_requerida: 'endodontic',
  implante: 'prosthetic',
  protesis_fija: 'prosthetic',
  protesis_removible: 'prosthetic',
  exodoncia: 'extraction',
  sellante: 'sealant',
}

export default function ToothSurfaceDiagram({ tooth, condition, selected, onToggle }: {
  tooth: string
  condition: string
  selected: string[]
  onToggle: (surface: string) => void
}) {
  const quadrant = Number(tooth[0])
  const palate = quadrant === 1 || quadrant === 2
  const frontTooth = Number(tooth[1]) <= 3
  const surfaces = [
    { id: 'vestibular', label: 'V', title: 'Vestibular', position: 'top' },
    { id: 'mesial', label: 'M', title: 'Mesial', position: 'left' },
    { id: frontTooth ? 'incisal' : 'oclusal', label: frontTooth ? 'I' : 'O', title: frontTooth ? 'Incisal' : 'Oclusal', position: 'center' },
    { id: 'distal', label: 'D', title: 'Distal', position: 'right' },
    { id: palate ? 'palatino' : 'lingual', label: palate ? 'P' : 'L', title: palate ? 'Palatino' : 'Lingual', position: 'bottom' },
  ]
  const color = surfaceStyle[condition] ?? 'other'
  return <div className="tooth-surface-diagram" role="group" aria-label={`Superficies de la pieza ${tooth}`}>
    {surfaces.map(surface => <button
      type="button"
      key={surface.id}
      title={surface.title}
      aria-label={`${surface.title}${selected.includes(surface.id) ? ', seleccionada' : ''}`}
      aria-pressed={selected.includes(surface.id)}
      className={`tooth-surface ${surface.position} ${selected.includes(surface.id) ? color : ''}`}
      onClick={() => onToggle(surface.id)}
    >{surface.label}</button>)}
  </div>
}
