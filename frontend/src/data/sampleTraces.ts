/**
 * Illustrative sample data for the landing page's monitor bank.
 * Not live — the Go agent's REST API doesn't exist yet (see docs/architecture.md).
 */
export type SampleTrace = {
  namespace: string
  pod: string
  metric: string
  value: string
  status: 'stable' | 'alert'
  path: string
  duration: string
}

export const sampleTraces: SampleTrace[] = [
  {
    namespace: 'prod',
    pod: 'checkout-7f9d4c-x2v9k',
    metric: 'cpu',
    value: '38%',
    status: 'stable',
    path: 'M0,22 C15,18 25,24 40,20 C55,17 65,23 80,19 C95,16 105,22 120,20 C135,18 145,21 160,19 C175,17 185,23 200,20 C210,19 215,21 220,20',
    duration: '4.2s',
  },
  {
    namespace: 'prod',
    pod: 'checkout-7f9d4c-m1qz2',
    metric: 'cpu',
    value: '41%',
    status: 'stable',
    path: 'M0,20 C10,24 20,16 30,20 C45,23 55,17 65,20 C80,24 90,16 100,20 C115,23 125,17 135,20 C150,24 160,16 170,20 C185,23 195,17 205,20 C215,19 218,20 220,20',
    duration: '3.6s',
  },
  {
    namespace: 'prod',
    pod: 'payments-6c8b9f-k4jp1',
    metric: 'memory',
    value: '212% of baseline',
    status: 'alert',
    path: 'M0,20 C20,19 40,21 60,20 C75,20 85,20 92,20 L96,4 L100,36 L104,20 C115,19 130,21 145,20 C160,19 175,21 190,20 C200,19 210,21 220,20',
    duration: '2.4s',
  },
  {
    namespace: 'staging',
    pod: 'worker-queue-5d7f-h9n3',
    metric: 'memory',
    value: '54%',
    status: 'stable',
    path: 'M0,16 C15,18 25,14 35,17 C50,20 60,15 70,18 C85,21 95,17 105,20 C120,22 130,19 140,21 C155,22 165,20 175,21 C190,22 200,20.5 210,21 C215,21 218,21 220,21',
    duration: '5s',
  },
]
