// 全球流量地球 - 3D 可视化核心组件
import { useRef, useMemo } from 'react'
import { Canvas, useFrame } from '@react-three/fiber'
import { OrbitControls, Sphere, Line } from '@react-three/drei'
import * as THREE from 'three'

interface GatewayNode {
  id: string
  lat: number
  lng: number
  status: 'online' | 'offline' | 'threat' | 'incubation' | 'calibration'
  threatLevel: number
  phase?: number // 0=潜伏, 1=校准, 2=服役
  networkQuality?: number
}

interface ThreatEvent {
  gatewayId: string
  timestamp: number
  severity: number
}

interface Props {
  gateways: GatewayNode[]
  threats: ThreatEvent[]
  highlightCoords?: { lat: number; lng: number; timestamp: number }[]
}

// 经纬度转换为 3D 坐标
function latLngToVector3(lat: number, lng: number, radius: number = 2): THREE.Vector3 {
  const phi = (90 - lat) * (Math.PI / 180)
  const theta = (lng + 180) * (Math.PI / 180)
  
  const x = -(radius * Math.sin(phi) * Math.cos(theta))
  const z = radius * Math.sin(phi) * Math.sin(theta)
  const y = radius * Math.cos(phi)
  
  return new THREE.Vector3(x, y, z)
}

// 地球本体
function Earth() {
  const earthRef = useRef<THREE.Mesh>(null)
  
  useFrame(() => {
    if (earthRef.current) {
      earthRef.current.rotation.y += 0.001 // 缓慢自转
    }
  })
  
  return (
    <Sphere ref={earthRef} args={[2, 64, 64]}>
      <meshStandardMaterial
        color="#0a1929"
        roughness={0.8}
        metalness={0.2}
        wireframe={false}
      />
    </Sphere>
  )
}

// Gateway 节点标记
function GatewayMarker({ gateway }: { gateway: GatewayNode }) {
  const position = useMemo(
    () => latLngToVector3(gateway.lat, gateway.lng, 2.05),
    [gateway.lat, gateway.lng]
  )
  
  const color = useMemo(() => {
    // 优先显示生命周期状态
    if (gateway.phase !== undefined) {
      switch (gateway.phase) {
        case 0: return '#9ca3af' // 灰色 - 潜伏期
        case 1: return '#fbbf24' // 黄色 - 校准期
        case 2: return '#10b981' // 绿色 - 服役期
      }
    }
    
    // 兼容旧状态
    switch (gateway.status) {
      case 'threat': return '#ef4444' // 红色 - 威胁
      case 'online': return '#10b981' // 绿色 - 在线
      case 'offline': return '#6b7280' // 灰色 - 离线
      case 'incubation': return '#9ca3af' // 灰色 - 潜伏
      case 'calibration': return '#fbbf24' // 黄色 - 校准
    }
  }, [gateway.status, gateway.phase])
  
  const markerRef = useRef<THREE.Mesh>(null)
  
  useFrame((state) => {
    if (markerRef.current && gateway.status === 'threat') {
      // 威胁节点脉冲动画
      const scale = 1 + Math.sin(state.clock.elapsedTime * 3) * 0.3
      markerRef.current.scale.setScalar(scale)
    }
  })
  
  return (
    <mesh ref={markerRef} position={position}>
      <sphereGeometry args={[0.02, 16, 16]} />
      <meshBasicMaterial color={color} />
    </mesh>
  )
}

// 威胁波纹效果
function ThreatRipple({ gateway, timestamp }: { gateway: GatewayNode; timestamp: number }) {
  const position = useMemo(
    () => latLngToVector3(gateway.lat, gateway.lng, 2.1),
    [gateway.lat, gateway.lng]
  )
  
  const rippleRef = useRef<THREE.Mesh>(null)
  
  useFrame((state) => {
    if (rippleRef.current) {
      const elapsed = state.clock.elapsedTime * 1000 - timestamp
      const progress = Math.min(elapsed / 2000, 1) // 2 秒动画
      
      if (progress < 1) {
        rippleRef.current.scale.setScalar(1 + progress * 2)
        const material = rippleRef.current.material as THREE.MeshBasicMaterial
        material.opacity = 1 - progress
      } else {
        rippleRef.current.visible = false
      }
    }
  })
  
  return (
    <mesh ref={rippleRef} position={position}>
      <ringGeometry args={[0.05, 0.08, 32]} />
      <meshBasicMaterial color="#ef4444" transparent opacity={1} side={THREE.DoubleSide} />
    </mesh>
  )
}

// 流量连线（Gateway → 中心）
function TrafficLine({ gateway }: { gateway: GatewayNode }) {
  const start = useMemo(
    () => latLngToVector3(gateway.lat, gateway.lng, 2.05),
    [gateway.lat, gateway.lng]
  )
  const end = new THREE.Vector3(0, 0, 0)
  
  const points = useMemo(() => {
    const curve = new THREE.QuadraticBezierCurve3(
      start,
      start.clone().multiplyScalar(0.5),
      end
    )
    return curve.getPoints(50)
  }, [start, end])
  
  return (
    <Line
      points={points}
      color="#3b82f6"
      lineWidth={1}
      transparent
      opacity={0.3}
    />
  )
}

// 高危坐标脉冲
function HighlightPulse({ lat, lng }: { lat: number; lng: number }) {
  const position = useMemo(
    () => latLngToVector3(lat, lng, 2.15),
    [lat, lng]
  )
  
  const pulseRef = useRef<THREE.Mesh>(null)
  
  useFrame((state) => {
    if (pulseRef.current) {
      const scale = 1 + Math.sin(state.clock.elapsedTime * 5) * 0.5
      pulseRef.current.scale.setScalar(scale)
    }
  })
  
  return (
    <mesh ref={pulseRef} position={position}>
      <sphereGeometry args={[0.06, 16, 16]} />
      <meshBasicMaterial color="#ff0000" transparent opacity={0.7} />
    </mesh>
  )
}

// 主组件
export default function GlobalTrafficGlobe({ gateways, threats, highlightCoords = [] }: Props) {
  const recentThreats = useMemo(() => {
    const now = Date.now()
    return threats.filter(t => now - t.timestamp < 5000) // 最近 5 秒
  }, [threats])
  
  return (
    <div className="w-full h-full bg-slate-950">
      <Canvas camera={{ position: [0, 0, 5], fov: 50 }}>
        <ambientLight intensity={0.3} />
        <pointLight position={[10, 10, 10]} intensity={0.8} />
        
        {/* 地球 */}
        <Earth />
        
        {/* Gateway 节点 */}
        {gateways.map(gateway => (
          <GatewayMarker key={gateway.id} gateway={gateway} />
        ))}
        
        {/* 流量连线 */}
        {gateways.filter(g => g.status === 'online').map(gateway => (
          <TrafficLine key={`line-${gateway.id}`} gateway={gateway} />
        ))}
        
        {/* 威胁波纹 */}
        {recentThreats.map(threat => {
          const gateway = gateways.find(g => g.id === threat.gatewayId)
          return gateway ? (
            <ThreatRipple
              key={`ripple-${threat.gatewayId}-${threat.timestamp}`}
              gateway={gateway}
              timestamp={threat.timestamp}
            />
          ) : null
        })}
        
        {/* 高危坐标脉冲 */}
        {highlightCoords.map((coord, idx) => (
          <HighlightPulse key={`pulse-${coord.timestamp}-${idx}`} lat={coord.lat} lng={coord.lng} />
        ))}
        
        <OrbitControls
          enableZoom={true}
          enablePan={false}
          minDistance={3}
          maxDistance={10}
          autoRotate
          autoRotateSpeed={0.5}
        />
      </Canvas>
      
      {/* 图例 */}
      <div className="absolute bottom-4 left-4 bg-slate-900/80 backdrop-blur-sm p-4 rounded-lg text-white text-sm">
        <div className="space-y-2">
          <div className="flex items-center gap-2">
            <div className="w-3 h-3 rounded-full bg-gray-400" />
            <span>潜伏期 (Incubation)</span>
          </div>
          <div className="flex items-center gap-2">
            <div className="w-3 h-3 rounded-full bg-yellow-400" />
            <span>校准期 (Calibration)</span>
          </div>
          <div className="flex items-center gap-2">
            <div className="w-3 h-3 rounded-full bg-green-500" />
            <span>服役期 (Active)</span>
          </div>
          <div className="flex items-center gap-2">
            <div className="w-3 h-3 rounded-full bg-red-500 animate-pulse" />
            <span>威胁检测</span>
          </div>
        </div>
      </div>
    </div>
  )
}
