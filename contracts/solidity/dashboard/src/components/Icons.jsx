import React from 'react'
import PropTypes from 'prop-types'

const Keep = ({ color, height, width }) => (
  <svg height={height} id="svg2" width={width} version="1.1" viewBox="0 0 928.81335 238.16" xmlSpace="preserve">
    <defs id="defs6"/>
    <g id="g10" transform="matrix(1.3333333,0,0,-1.3333333,0,238.16)">
      <g id="g12" transform="scale(0.1)">
        <path id="path14" style={{ 'fill': color, 'fillOpacity': '1', 'fillRule': 'nonzero', 'stroke': 'none' }} d="m 1959.7,1388.16 v 398.07 h 1589.69 v -650.74 h -446.54 v 257.75 h -431.21 v -303.63 h 370.01 V 722.191 H 2671.64 V 392.988 h 438.88 v 290.883 h 446.54 V 0 H 1959.7 v 398.059 h 188.79 V 1388.16 H 1959.7"/>
        <path id="path16" style={{ 'fill': color, 'fillOpacity': '1', 'fillRule': 'nonzero', 'stroke': 'none' }} d="m 3651.49,1388.16 v 398.07 h 1589.7 v -650.74 h -446.55 v 257.75 h -431.21 v -303.63 h 370.01 V 722.191 H 4363.43 V 392.988 h 438.88 v 290.883 h 446.53 V 0 H 3651.49 v 398.059 h 188.79 v 990.101 h -188.79"/>
        <path id="path18" style={{ 'fill': color, 'fillOpacity': '1', 'fillRule': 'nonzero', 'stroke': 'none' }} d="m 6274.66,1005.42 c 73.95,0 102.01,5.08 117.34,20.4 22.98,25.48 28.06,63.79 28.06,173.46 0,109.77 -5.08,147.98 -28.06,173.56 -15.33,15.32 -43.39,20.4 -117.34,20.4 H 6055.23 V 1005.42 Z M 6851.37,724.691 C 6782.41,653.23 6675.23,627.75 6517.09,627.75 H 6055.23 V 398.059 h 201.53 V 0 h -913.47 v 398.059 h 188.78 v 990.101 h -188.78 v 398.07 h 1173.8 c 158.14,0 265.32,-25.57 334.28,-97.02 81.6,-84.2 114.75,-173.46 114.75,-482.27 0,-308.706 -33.15,-398.065 -114.75,-482.249"/>
        <path id="path20" style={{ 'fill': color, 'fillOpacity': '1', 'fillRule': 'nonzero', 'stroke': 'none' }} d="m 1865.26,1388.16 v 398.07 H 964.527 V 1393.24 H 1104.84 L 826.695,1069.21 H 711.949 v 324.03 h 158.133 v 392.99 H 656.352 V 1638.86 H 540.684 v 147.37 H 329.43 V 1638.86 H 213.754 v 147.37 H 0 V 1388.16 H 188.789 V 398.059 L 0,398.059 V 0 H 870.082 V 392.988 H 711.949 V 717.02 H 826.695 L 1104.84,392.988 H 964.527 V 0 h 900.733 v 398.059 h -140.32 l -423.99,495.054 423.99,495.047 h 140.32"/>
      </g>
    </g>
  </svg>
)

Keep.propTypes = {
  color: PropTypes.string,
  height: PropTypes.string,
  width: PropTypes.string,
}

Keep.defaultProps = {
  color: '#293330',
  height: '238.16',
  width: '917.41333',
}

export {
  Keep,
}
