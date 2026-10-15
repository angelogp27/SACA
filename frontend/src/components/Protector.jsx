// Protector atrapa un error al dibujar una parte de la página y muestra el
// mensaje con un botón para reintentar, en vez de dejar toda la página en
// blanco (o en negro, en modo oscuro) en plena presentación.
import { Component } from 'react'

export default class Protector extends Component {
  constructor(props) {
    super(props)
    this.state = { error: null }
  }

  static getDerivedStateFromError(error) {
    return { error }
  }

  componentDidCatch(error, info) {
    console.error('Error al dibujar la página:', error, info?.componentStack)
  }

  render() {
    if (!this.state.error) return this.props.children
    return (
      <div className="panel protector" role="alert">
        <h3>Algo falló al dibujar esta parte</h3>
        <p>{String(this.state.error?.message ?? this.state.error)}</p>
        <button className="boton-mini" onClick={() => this.setState({ error: null })}>Reintentar</button>
      </div>
    )
  }
}
