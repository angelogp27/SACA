// Mapa 3D de EE. UU. continental con three.js: cada estado es una placa con
// sus bordes, el contorno del país más grueso y cada estación una columna
// (alto = su valor, color = su nivel). Se gira arrastrando, se acerca con la
// rueda y se mueve con clic derecho. Solo dibuja cuando algo cambia, para no
// gastar CPU mientras el clúster calcula.
import { useEffect, useRef } from 'react'
import * as THREE from 'three'
import { OrbitControls } from 'three/examples/jsm/controls/OrbitControls.js'
import { LineSegments2 } from 'three/examples/jsm/lines/LineSegments2.js'
import { LineSegmentsGeometry } from 'three/examples/jsm/lines/LineSegmentsGeometry.js'
import { LineMaterial } from 'three/examples/jsm/lines/LineMaterial.js'
import { feature, mesh } from 'topojson-client'
import atlas from 'us-atlas/states-10m.json'

// Fuera del continente: Alaska, Hawái y los territorios.
const FUERA = new Set(['02', '15', '60', '66', '69', '72', '78'])
const LON0 = -96
const LAT0 = 38
const COS = Math.cos((LAT0 * Math.PI) / 180)
const GROSOR = 0.35 // de las placas de los estados
const ALTO_MAX = 5 // de la columna más alta


const proyectar = ([lon, lat]) => [(lon - LON0) * COS, lat - LAT0]
const continental = (anillo) => {
  const [lon, lat] = anillo[0]
  return lon > -130 && lon < -60 && lat > 23 && lat < 51
}

// Geometrías fijas (no dependen de los datos): se arman una sola vez.
let base = null
function geometriaBase() {
  if (base) return base
  const estados = feature(atlas, atlas.objects.states).features.filter((f) => !FUERA.has(f.id))
  const formas = []
  for (const f of estados) {
    const poligonos = f.geometry.type === 'Polygon' ? [f.geometry.coordinates] : f.geometry.coordinates
    for (const [exterior, ...huecos] of poligonos) {
      const forma = new THREE.Shape(exterior.map((p) => new THREE.Vector2(...proyectar(p))))
      forma.holes = huecos.map((h) => new THREE.Path(h.map((p) => new THREE.Vector2(...proyectar(p)))))
      formas.push(forma)
    }
  }
  const tierra = new THREE.ExtrudeGeometry(formas, { depth: GROSOR, bevelEnabled: false })
  const segmentos = (multilinea) => {
    const pos = []
    for (const linea of multilinea.coordinates) {
      if (!continental(linea)) continue
      for (let i = 1; i < linea.length; i++) {
        pos.push(...proyectar(linea[i - 1]), GROSOR + 0.01, ...proyectar(linea[i]), GROSOR + 0.01)
      }
    }
    return new LineSegmentsGeometry().setPositions(pos)
  }
  const internos = segmentos(mesh(atlas, atlas.objects.states, (a, b) => a !== b && !FUERA.has(a.id) && !FUERA.has(b.id)))
  const pais = segmentos(mesh(atlas, atlas.objects.nation))
  base = { tierra, internos, pais }
  return base
}

const css = (nombre) => getComputedStyle(document.documentElement).getPropertyValue(nombre).trim() || '#888'

export default function Mapa3D({ estaciones, valor, colorDe, seleccionada, onSeleccionar, onHover, etiqueta, reiniciar }) {
  const caja = useRef(null)
  const escena = useRef(null)
  // Lo último de las props, para los manejadores de eventos de three.
  const props = useRef({})
  useEffect(() => {
    props.current = { onSeleccionar, onHover }
  })

  // La escena: una sola vez.
  useEffect(() => {
    const el = caja.current
    const { tierra, internos, pais } = geometriaBase()
    const renderer = new THREE.WebGLRenderer({ antialias: true, alpha: true })
    renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2))
    el.appendChild(renderer.domElement)
    const scene = new THREE.Scene()
    const camara = new THREE.PerspectiveCamera(35, 1, 0.1, 500)
    const inicio = new THREE.Vector3(0, -40, 27)
    camara.position.copy(inicio)
    camara.up.set(0, 0, 1)
    const controles = new OrbitControls(camara, renderer.domElement)
    controles.target.set(0, 1.5, 0)
    controles.update()
    controles.maxPolarAngle = Math.PI / 2.2
    controles.minDistance = 10
    controles.maxDistance = 110
    controles.enableDamping = false

    scene.add(new THREE.HemisphereLight(0xffffff, 0x666666, 2.2))
    const sol = new THREE.DirectionalLight(0xffffff, 1.4)
    sol.position.set(-20, -30, 40)
    scene.add(sol)

    const matTierra = new THREE.MeshStandardMaterial({ roughness: 0.9, metalness: 0 })
    scene.add(new THREE.Mesh(tierra, matTierra))
    const matInternos = new LineMaterial({ linewidth: 1 })
    const matPais = new LineMaterial({ linewidth: 2.6 })
    scene.add(new LineSegments2(internos, matInternos))
    scene.add(new LineSegments2(pais, matPais))

    const columna = new THREE.CylinderGeometry(0.13, 0.13, 1, 10)
    columna.rotateX(Math.PI / 2)
    columna.translate(0, 0, 0.5) // la base sobre la placa
    const matColumnas = new THREE.MeshStandardMaterial({ roughness: 0.5 })
    const anillo = new THREE.Mesh(new THREE.RingGeometry(0.32, 0.46, 24), new THREE.MeshBasicMaterial({ side: THREE.DoubleSide }))
    anillo.visible = false
    scene.add(anillo)

    const tema = () => {
      matTierra.color.set(css('--gridline'))
      matInternos.color.set(css('--text-muted'))
      matPais.color.set(css('--text-primary'))
      anillo.material.color.set(css('--text-primary'))
    }
    tema()

    let columnas = null
    const raton = new THREE.Vector2()
    const rayo = new THREE.Raycaster()
    let pendiente = false
    const dibujar = () => {
      if (pendiente) return
      pendiente = true
      requestAnimationFrame(() => {
        pendiente = false
        renderer.render(scene, camara)
      })
    }
    const medir = () => {
      const ancho = el.clientWidth
      const alto = Math.max(260, Math.round(ancho * 0.56))
      renderer.setSize(ancho, alto)
      camara.aspect = ancho / alto
      camara.updateProjectionMatrix()
      matInternos.resolution.set(ancho, alto)
      matPais.resolution.set(ancho, alto)
      dibujar()
    }
    const tocada = (ev) => {
      if (!columnas) return null
      const r = renderer.domElement.getBoundingClientRect()
      raton.set(((ev.clientX - r.left) / r.width) * 2 - 1, -((ev.clientY - r.top) / r.height) * 2 + 1)
      rayo.setFromCamera(raton, camara)
      const hit = rayo.intersectObject(columnas)[0]
      return hit ? columnas.userData.lista[hit.instanceId] : null
    }
    let ultimaHover = null
    const mover = (ev) => {
      const e = tocada(ev)
      if (e !== ultimaHover) {
        ultimaHover = e
        renderer.domElement.style.cursor = e ? 'pointer' : 'grab'
        props.current.onHover(e)
      }
    }
    // Un clic (no un arrastre) elige la estación bajo el cursor.
    let abajo = null
    const presionar = (ev) => { abajo = [ev.clientX, ev.clientY] }
    const soltar = (ev) => {
      if (!abajo || Math.hypot(ev.clientX - abajo[0], ev.clientY - abajo[1]) > 4) return
      const e = tocada(ev)
      if (e) props.current.onSeleccionar(e.id)
    }
    const salir = () => {
      ultimaHover = null
      props.current.onHover(null)
    }
    renderer.domElement.addEventListener('pointermove', mover)
    renderer.domElement.addEventListener('pointerdown', presionar)
    renderer.domElement.addEventListener('pointerup', soltar)
    renderer.domElement.addEventListener('pointerleave', salir)
    controles.addEventListener('change', dibujar)
    const obs = new ResizeObserver(medir)
    obs.observe(el)
    const oscuro = window.matchMedia('(prefers-color-scheme: dark)')
    const cambioTema = () => {
      tema()
      dibujar()
    }
    oscuro.addEventListener('change', cambioTema)
    medir()

    escena.current = {
      dibujar,
      reiniciar: () => {
        camara.position.copy(inicio)
        controles.target.set(0, 1.5, 0)
        controles.update()
      },
      // Las columnas cambian con los datos: se rehacen enteras.
      columnas: (lista, alto, color) => {
        if (columnas) {
          scene.remove(columnas)
          columnas.dispose()
        }
        columnas = new THREE.InstancedMesh(columna, matColumnas, Math.max(lista.length, 1))
        columnas.count = lista.length
        const m = new THREE.Matrix4()
        const c = new THREE.Color()
        lista.forEach((e, i) => {
          const [x, y] = proyectar([e.lon, e.lat])
          m.makeScale(1, 1, alto(e))
          m.setPosition(x, y, GROSOR)
          columnas.setMatrixAt(i, m)
          columnas.setColorAt(i, c.set(color(e)))
        })
        columnas.userData.lista = lista
        scene.add(columnas)
        dibujar()
      },
      marcar: (e) => {
        anillo.visible = !!e
        if (e) {
          const [x, y] = proyectar([e.lon, e.lat])
          anillo.position.set(x, y, GROSOR + 0.02)
        }
        dibujar()
      },
    }
    return () => {
      obs.disconnect()
      oscuro.removeEventListener('change', cambioTema)
      controles.dispose()
      if (columnas) columnas.dispose()
      columna.dispose()
      matColumnas.dispose()
      matTierra.dispose()
      matInternos.dispose()
      matPais.dispose()
      anillo.geometry.dispose()
      anillo.material.dispose()
      renderer.dispose()
      el.removeChild(renderer.domElement)
      escena.current = null
    }
  }, [])

  // Las columnas: alto relativo al mayor valor de la lista.
  useEffect(() => {
    const max = Math.max(1e-9, ...estaciones.map(valor))
    const colores = {}
    const color = (e) => {
      const v = colorDe(e)
      if (!(v in colores)) colores[v] = css(v.replace(/^var\((.*)\)$/, '$1'))
      return colores[v]
    }
    escena.current?.columnas(estaciones, (e) => 0.15 + (ALTO_MAX * Math.max(0, valor(e))) / max, color)
  }, [estaciones, valor, colorDe])

  useEffect(() => {
    escena.current?.marcar(estaciones.find((e) => e.id === seleccionada))
  }, [estaciones, seleccionada])

  useEffect(() => {
    if (reiniciar) escena.current?.reiniciar()
  }, [reiniciar])

  return <div ref={caja} className="mapa-3d" role="img" aria-label={etiqueta} />
}
