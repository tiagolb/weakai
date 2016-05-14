package neuralnet

import (
	"encoding/json"
	"math"
	"math/rand"

	"github.com/unixpickle/num-analysis/kahan"
)

// DenseParams are parameters for a dense
// or "fully-connected" layer.
type DenseParams struct {
	Activation  ActivationFunc
	InputCount  int
	OutputCount int
}

// Make creates a *DenseLayer based on the
// parameters specified by p.
// This is equivalent to NewDenseLayer(p).
func (p *DenseParams) Make() Layer {
	return NewDenseLayer(p)
}

type DenseLayer struct {
	activation ActivationFunc

	// Each weight list or bias value corresponds
	// to a neuron (and thus to an output).
	weights [][]float64
	biases  []float64

	output     []float64
	outputSums []float64

	downstreamGradient []float64
	weightGradient     [][]float64
	biasGradient       []float64

	upstreamGradient []float64
	input            []float64
}

func NewDenseLayer(params *DenseParams) *DenseLayer {
	res := &DenseLayer{
		activation:       params.Activation,
		weights:          make([][]float64, params.OutputCount),
		biases:           make([]float64, params.OutputCount),
		output:           make([]float64, params.OutputCount),
		weightGradient:   make([][]float64, params.OutputCount),
		biasGradient:     make([]float64, params.OutputCount),
		upstreamGradient: make([]float64, params.InputCount),
		outputSums:       make([]float64, params.OutputCount),
	}
	for i := range res.weights {
		res.weights[i] = make([]float64, params.InputCount)
		res.weightGradient[i] = make([]float64, params.InputCount)
	}
	return res
}

func DeserializeDenseLayer(data []byte) (*DenseLayer, error) {
	var s serializedDenseLayer
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}

	activation, err := deserializeActivation(s.ActivationData, s.ActivationType)
	if err != nil {
		return nil, err
	}

	res := &DenseLayer{
		activation:       activation,
		weights:          s.Weights,
		biases:           s.Biases,
		output:           make([]float64, s.OutputSize),
		weightGradient:   make([][]float64, s.OutputSize),
		biasGradient:     make([]float64, s.OutputSize),
		upstreamGradient: make([]float64, s.InputSize),
		outputSums:       make([]float64, s.OutputSize),
	}

	for i := range res.weights {
		res.weightGradient[i] = make([]float64, s.InputSize)
	}

	return res, nil
}

// Weights returns a list of weight slices,
// where each slice corresponds to a hidden
// neuron.
// The caller should not modify the result.
func (d *DenseLayer) Weights() [][]float64 {
	return d.weights
}

// Biases returns a list of biases, one
// for each hidden neuron.
func (d *DenseLayer) Biases() []float64 {
	return d.biases
}

// Randomize randomizes the weights and biases.
// The biases are chosen uniformly such that
// their variance is 1.
// The weights are chosen uniformly such that
// the variance of the sum of all the weights
// for a given neuron is 1.
func (d *DenseLayer) Randomize() {
	sqrt3 := math.Sqrt(3)
	for i := range d.biases {
		d.biases[i] = sqrt3 * ((rand.Float64() * 2) - 1)
	}
	weightCoeff := math.Sqrt(3.0 / float64(len(d.upstreamGradient)))
	for _, weights := range d.weights {
		for i := range weights {
			weights[i] = weightCoeff * ((rand.Float64() * 2) - 1)
		}
	}
}

func (d *DenseLayer) PropagateForward() {
	for i, weights := range d.weights {
		sum := kahan.NewSummer64()
		for j, weight := range weights {
			sum.Add(weight * d.input[j])
		}
		sum.Add(d.biases[i])
		d.outputSums[i] = sum.Sum()
		d.output[i] = d.activation.Eval(sum.Sum())
	}
}

func (d *DenseLayer) PropagateBackward(upstream bool) {
	if upstream {
		for i := range d.upstreamGradient {
			d.upstreamGradient[i] = 0
		}
	}

	for i, weights := range d.weights {
		sumPartial := d.downstreamGradient[i] * d.activation.Deriv(d.outputSums[i])
		d.biasGradient[i] = sumPartial
		for j, weight := range weights {
			d.weightGradient[i][j] = d.input[j] * sumPartial
			if upstream {
				d.upstreamGradient[j] += sumPartial * weight
			}
		}
	}
}

func (d *DenseLayer) GradientMagSquared() float64 {
	sum := kahan.NewSummer64()
	for _, x := range d.biasGradient {
		sum.Add(x * x)
	}

	for _, weightGrads := range d.weightGradient {
		for _, grad := range weightGrads {
			sum.Add(grad * grad)
		}
	}

	return sum.Sum()
}

func (d *DenseLayer) StepGradient(f float64) {
	for i, x := range d.biasGradient {
		d.biases[i] += x * f
	}
	for i, weightGrads := range d.weightGradient {
		for j, grad := range weightGrads {
			d.weights[i][j] += grad * f
		}
	}
}

func (d *DenseLayer) Output() []float64 {
	return d.output
}

func (d *DenseLayer) UpstreamGradient() []float64 {
	return d.upstreamGradient
}

func (d *DenseLayer) Input() []float64 {
	return d.input
}

func (d *DenseLayer) SetInput(v []float64) bool {
	if len(v) != len(d.upstreamGradient) {
		return false
	}
	d.input = v
	return true
}

func (d *DenseLayer) DownstreamGradient() []float64 {
	return d.downstreamGradient
}

func (d *DenseLayer) SetDownstreamGradient(v []float64) bool {
	if len(v) != len(d.output) {
		return false
	}
	d.downstreamGradient = v
	return true
}

func (d *DenseLayer) Serialize() []byte {
	s := serializedDenseLayer{
		ActivationData: d.activation.Serialize(),
		ActivationType: d.activation.SerializerType(),

		Weights:    d.weights,
		Biases:     d.biases,
		InputSize:  len(d.upstreamGradient),
		OutputSize: len(d.output),
	}
	data, _ := json.Marshal(&s)
	return data
}

func (d *DenseLayer) SerializerType() string {
	return "denselayer"
}

type serializedDenseLayer struct {
	ActivationData []byte
	ActivationType string

	Weights [][]float64
	Biases  []float64

	InputSize  int
	OutputSize int
}